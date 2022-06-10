# littleraft :boat:

## What is this?

Yet another implementation of the raft algorithm in Golang, mostly for learning purposes.

## Usage 


This project is yet to implement some of the finer components of raft, such as crash persistence and log compaction, hence the name "littleraft". It offers the bare bones protocol for consensus in `raft.go`.

Go version 1.16+ is needed to build the project and you can run a handful of tests with `go test` in your shell of choice.

## What is Raft?

Raft is a consensus algorithm that is designed to be easy to understand.

Consensus is a fundamental problem in fault-tolerant distributed systems. Consensus involves multiple servers agreeing on values. Once they reach a decision on a value, that decision is final. Typical consensus algorithms make progress when any majority of their servers is available; for example, a cluster of 5 servers can continue to operate even if 2 servers fail. If more servers fail, they stop making progress (but will never return an incorrect result).

For more details on the algorithm you can reference the [Raft paper](https://raft.github.io/raft.pdf) 

Some other resources include one of the authors Diego Ongario's [website](https://raft.github.io/) as well as this [visualization](http://thesecretlivesofdata.com/raft/) of the algorithm.
