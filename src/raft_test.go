package raft

import (
	"testing"
)

// 1
func TestElectionBasic(t *testing.T) {
	test := StartTest(t, 3)
	defer test.StopTest()
	test.CheckSingleLeader()
}

// 2
func TestDisconnectAllThenRestore(t *testing.T) {
	test := StartTest(t, 3)
	defer test.StopTest()
	for i := 0; i < 3; i++ {
		test.DisconnectPeer(i)
	}
	test.CheckNoLeader()
	for i := 0; i < 3; i++ {
		test.ReconnectPeer(i)
	}
	test.CheckSingleLeader()
}

// 3
func TestElectionLeaderAndAnotherDisconnect(t *testing.T) {
	test := StartTest(t, 3)
	defer test.StopTest()

	origLeaderId, _ := test.CheckSingleLeader()
	test.DisconnectPeer(origLeaderId)
	otherId := (origLeaderId + 1) % 3
	test.DisconnectPeer(otherId)
	sleep(450)
	test.CheckNoLeader()
	test.ReconnectPeer(otherId)
	test.CheckSingleLeader()
}

// 4
func TestSubmitNonLeaderFails(t *testing.T) {
	h := StartTest(t, 3)
	defer h.StopTest()
	origLeaderId, _ := h.CheckSingleLeader()
	sid := (origLeaderId + 1) % 3
	testlog("submitting 45 to %d", sid)
	isLeader := h.SubmitToServer(sid, 45)
	if isLeader {
		t.Errorf("node %d should not be the leader", sid)
	}
}
