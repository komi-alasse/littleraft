package raft

import (
	"math/rand"
	"sync"
	"time"
)

const DEBUG bool = false

type Commit struct {
	Command interface{}
	Index   int
	Term    int
}

type State int

const (
	Follower State = iota
	Candidate
	Leader
	Dead
)

type LogEntry struct {
	Command interface{}
	Term    int
}

type Raft struct {
	id                 int
	currentTerm        int
	votedFor           int
	commitIndex        int
	lastApplied        int
	peerIds            []int
	nextIndex          map[int]int
	matchIndex         map[int]int
	mu                 sync.Mutex
	server             *Server
	newCommit          chan struct{}
	log                []LogEntry
	state              State
	electionResetEvent time.Time
}

func NewRaft(id int, peerIds []int, server *Server) *Raft {
	raft := new(Raft)
	raft.id = id
	raft.peerIds = peerIds
	raft.server = server
	raft.newCommit = make(chan struct{})
	raft.state = Follower
	raft.nextIndex = make(map[int]int)
	raft.matchIndex = make(map[int]int)
	go func() {
		raft.mu.Lock()
		raft.electionResetEvent = time.Now()
		raft.mu.Unlock()
		raft.runElectionTimer()
	}()
	return raft
}

func (raft *Raft) Report() (id int, term int, isLeader bool) {
	raft.mu.Lock()
	defer raft.mu.Unlock()
	return raft.id, raft.currentTerm, raft.state == Leader
}

func (raft *Raft) Submit(command interface{}) bool {
	raft.mu.Lock()
	defer raft.mu.Unlock()
	if raft.state == Leader {
		raft.log = append(raft.log, LogEntry{Command: command, Term: raft.currentTerm})
		return true
	}
	return false
}

func (raft *Raft) Stop() {
	raft.mu.Lock()
	defer raft.mu.Unlock()
	raft.state = Dead
	close(raft.newCommit)
}

type Ballot struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

type VoteReply struct {
	Term        int
	VoteGranted bool
}

func (raft *Raft) RequestVote(args Ballot, reply *VoteReply) error {
	raft.mu.Lock()
	defer raft.mu.Unlock()
	if raft.state == Dead {
		return nil
	}
	lastLogIndex, lastLogTerm := raft.lastLogIndexAndTerm()

	if args.Term > raft.currentTerm {
		raft.Follow(args.Term)
	}

	if raft.currentTerm == args.Term &&
		(raft.votedFor == -1 || raft.votedFor == args.CandidateId) &&
		(args.LastLogTerm > lastLogTerm ||
			(args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)) {
		reply.VoteGranted = true
		raft.votedFor = args.CandidateId
		raft.electionResetEvent = time.Now()
	} else {
		reply.VoteGranted = false
	}
	reply.Term = raft.currentTerm
	return nil
}

// Figure 2 in Raft.
type EntryArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type EntryReply struct {
	Term    int
	Success bool
}

func (raft *Raft) AppendEntries(args EntryArgs, reply *EntryReply) error {
	raft.mu.Lock()
	defer raft.mu.Unlock()
	if raft.state == Dead {
		return nil
	}

	if args.Term > raft.currentTerm {
		raft.Follow(args.Term)
	}

	reply.Success = false
	if args.Term == raft.currentTerm {
		if raft.state != Follower {
			raft.Follow(args.Term)
		}
		raft.electionResetEvent = time.Now()

		if args.PrevLogIndex == -1 ||
			(args.PrevLogIndex < len(raft.log) && args.PrevLogTerm == raft.log[args.PrevLogIndex].Term) {
			reply.Success = true

			logInsertIndex := args.PrevLogIndex + 1
			newEntriesIndex := 0

			for {
				if logInsertIndex >= len(raft.log) || newEntriesIndex >= len(args.Entries) {
					break
				}
				if raft.log[logInsertIndex].Term != args.Entries[newEntriesIndex].Term {
					break
				}
				logInsertIndex++
				newEntriesIndex++
			}
			if newEntriesIndex < len(args.Entries) {
				raft.log = append(raft.log[:logInsertIndex], args.Entries[newEntriesIndex:]...)
			}

			if args.LeaderCommit > raft.commitIndex {
				raft.commitIndex = intMin(args.LeaderCommit, len(raft.log)-1)
				raft.newCommit <- struct{}{}
			}
		}
	}

	reply.Term = raft.currentTerm
	return nil
}

func (raft *Raft) electionTimeout() time.Duration {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
}

func (raft *Raft) runElectionTimer() {
	timeoutDuration := raft.electionTimeout()
	raft.mu.Lock()
	termStarted := raft.currentTerm
	raft.mu.Unlock()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		<-ticker.C
		raft.mu.Lock()
		if raft.state != Candidate && raft.state != Follower {
			raft.mu.Unlock()
			return
		}

		if termStarted != raft.currentTerm {
			raft.mu.Unlock()
			return
		}

		if elapsed := time.Since(raft.electionResetEvent); elapsed >= timeoutDuration {
			raft.startElection()
			raft.mu.Unlock()
			return
		}
		raft.mu.Unlock()
	}
}

func (raft *Raft) startElection() {
	raft.state = Candidate
	raft.currentTerm += 1
	savedCurrentTerm := raft.currentTerm
	raft.electionResetEvent = time.Now()
	raft.votedFor = raft.id

	votesReceived := 1

	for _, peerId := range raft.peerIds {
		go func(peerId int) {
			raft.mu.Lock()
			savedLastLogIndex, savedLastLogTerm := raft.lastLogIndexAndTerm()
			raft.mu.Unlock()

			args := Ballot{
				Term:         savedCurrentTerm,
				CandidateId:  raft.id,
				LastLogIndex: savedLastLogIndex,
				LastLogTerm:  savedLastLogTerm,
			}

			var reply VoteReply
			if err := raft.server.Call(peerId, "Raft.RequestVote", args, &reply); err == nil {
				raft.mu.Lock()
				defer raft.mu.Unlock()

				if raft.state != Candidate {
					return
				}

				if reply.Term > savedCurrentTerm {
					raft.Follow(reply.Term)
					return
				} else if reply.Term == savedCurrentTerm {
					if reply.VoteGranted {
						votesReceived += 1
						if votesReceived*2 > len(raft.peerIds)+1 {
							// Election won by current node.
							raft.startLeader()
							return
						}
					}
				}
			}
		}(peerId)
	}

	go raft.runElectionTimer()
}

func (raft *Raft) Follow(term int) {
	raft.state = Follower
	raft.currentTerm = term
	raft.votedFor = -1
	raft.electionResetEvent = time.Now()

	go raft.runElectionTimer()
}

func (raft *Raft) startLeader() {
	raft.state = Leader

	for _, peerId := range raft.peerIds {
		raft.nextIndex[peerId] = len(raft.log)
		raft.matchIndex[peerId] = -1
	}

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		// Send periodic heartbeats, as long as still leader.
		for {
			raft.SendHeartbeats()
			<-ticker.C

			raft.mu.Lock()
			if raft.state != Leader {
				raft.mu.Unlock()
				return
			}
			raft.mu.Unlock()
		}
	}()
}

func (raft *Raft) SendHeartbeats() {
	// Only the leader is allowed to do this.
	raft.mu.Lock()
	if raft.state != Leader {
		raft.mu.Unlock()
		return
	}
	savedCurrentTerm := raft.currentTerm
	raft.mu.Unlock()

	for _, peerId := range raft.peerIds {
		go func(peerId int) {
			raft.mu.Lock()
			newIndex := raft.nextIndex[peerId]
			prevLogIndex := newIndex - 1
			prevLogTerm := -1
			if prevLogIndex >= 0 {
				prevLogTerm = raft.log[prevLogIndex].Term
			}
			entries := raft.log[newIndex:]

			entry := EntryArgs{
				Term:         savedCurrentTerm,
				LeaderId:     raft.id,
				PrevLogIndex: prevLogIndex,
				PrevLogTerm:  prevLogTerm,
				Entries:      entries,
				LeaderCommit: raft.commitIndex,
			}
			raft.mu.Unlock()
			var reply EntryReply
			if err := raft.server.Call(peerId, "Raft.AppendEntries", entry, &reply); err == nil {
				raft.mu.Lock()
				defer raft.mu.Unlock()
				if reply.Term > raft.currentTerm {
					raft.Follow(reply.Term)
					return
				}

				if raft.state == Leader && savedCurrentTerm == reply.Term {
					if reply.Success {
						raft.nextIndex[peerId] = newIndex + len(entries)
						raft.matchIndex[peerId] = raft.nextIndex[peerId] - 1

						savedCommitIndex := raft.commitIndex
						for i := raft.commitIndex + 1; i < len(raft.log); i++ {
							if raft.log[i].Term == raft.currentTerm {
								matchCount := 1
								for _, peerId := range raft.peerIds {
									if raft.matchIndex[peerId] >= i {
										matchCount++
									}
								}
								if matchCount*2 > len(raft.peerIds)+1 {
									raft.commitIndex = i
								}
							}
						}
						if raft.commitIndex != savedCommitIndex {
							raft.newCommit <- struct{}{}
						}
					} else {
						raft.nextIndex[peerId] = newIndex - 1
					}
				}
			}
		}(peerId)
	}
}

func (raft *Raft) lastLogIndexAndTerm() (int, int) {
	if len(raft.log) > 0 {
		return len(raft.log) - 1, raft.log[len(raft.log)-1].Term
	}
	return -1, -1
}

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
