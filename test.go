package raft

import (
	"log"
	"sync"
	"testing"
	"time"
)

type Test struct {
	mu          sync.Mutex
	cluster     []*Server
	commitChans []chan Commit
	commits     [][]Commit
	connections []bool
	num         int
	test        *testing.T
}

func StartTest(t *testing.T, n int) *Test {
	servers := make([]*Server, n)
	conn := make([]bool, n)
	commitChans := make([]chan Commit, n)
	commits := make([][]Commit, n)

	// Create all Servers in this cluster, assign ids and peer ids.
	for i := 0; i < n; i++ {
		peerIds := make([]int, 0)
		for peerId := 0; peerId < n; peerId++ {
			if peerId != i {
				peerIds = append(peerIds, peerId)
			}
		}

		// Init servers.
		commitChans[i] = make(chan Commit)
		servers[i] = MakeServer(i, peerIds)
		servers[i].Serve()
	}

	// Connect all peers to each other.
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i != j {
				servers[i].ConnectToPeer(j, servers[j].GetListenAddr())
			}
		}
		conn[i] = true
	}

	test := &Test{
		cluster:     servers,
		commitChans: commitChans,
		commits:     commits,
		connections: conn,
		num:         n,
		test:        t,
	}
	return test
}

func (test *Test) StopTest() {
	for i := 0; i < test.num; i++ {
		test.cluster[i].DisconnectAll()
		test.connections[i] = false

	}
	for i := 0; i < test.num; i++ {
		test.cluster[i].Shutdown()
	}
	for i := 0; i < test.num; i++ {
		close(test.commitChans[i])
	}
}

func (test *Test) DisconnectPeer(id int) {
	testlog("Disconnecting %d", id)
	test.cluster[id].DisconnectAll()
	for j := 0; j < test.num; j++ {
		if j != id {
			test.cluster[j].DisconnectPeer(id)
		}
	}
	test.connections[id] = false
}

func (test *Test) ReconnectPeer(id int) {
	testlog("Reconnecting %d", id)
	for j := 0; j < test.num; j++ {
		if j != id {
			// Checks two way connections.
			if err := test.cluster[id].ConnectToPeer(j, test.cluster[j].GetListenAddr()); err != nil {
				test.test.Fatal(err)
			}
			if err := test.cluster[j].ConnectToPeer(id, test.cluster[id].GetListenAddr()); err != nil {
				test.test.Fatal(err)
			}
		}
	}
	test.connections[id] = true
}

func (test *Test) CheckSingleLeader() (int, int) {
	// Trys for leader can be adjusted.
	for try := 0; try < 5; try++ {
		leaderId := -1
		leaderTerm := -1
		for i := 0; i < test.num; i++ {
			if test.connections[i] {
				_, term, isLeader := test.cluster[i].raft.Report()
				if isLeader {
					if leaderId < 0 {
						leaderId = i
						leaderTerm = term
					} else {
						test.test.Fatalf("two leaders %d and %d", leaderId, i)
					}
				}
			}
		}
		if leaderId >= 0 {
			return leaderId, leaderTerm
		}
		time.Sleep(150 * time.Millisecond)
	}

	test.test.Fatalf("leader not found")
	return -1, -1
}

func (test *Test) CheckNoLeader() {
	for i := 0; i < test.num; i++ {
		if test.connections[i] {
			_, _, isLeader := test.cluster[i].raft.Report()
			if isLeader {
				test.test.Fatalf("leader at server %d", i)
			}
		}
	}
}

func (h *Test) SubmitToServer(serverId int, command interface{}) bool {
	return h.cluster[serverId].raft.Submit(command)
}

func testlog(format string, a ...interface{}) {
	format = "[TEST] " + format
	log.Printf(format, a...)
}

func sleep(n int) {
	time.Sleep(time.Duration(n) * time.Millisecond)
}
