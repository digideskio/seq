package rpc

import (
	"math/rand"
	"sync"

	"google.golang.org/grpc"
)

// -----------------------------------------------------------------------------

// Pool implements a generic, thread-safe round-robin based, pool of
// GRPC connections.
//
// All methods of a `Pool` are thread-safe.
type Pool struct {
	*sync.RWMutex
	conns   []*grpc.ClientConn
	current int
}

// NewPool returns a new `Pool` with one established connection
// for each specified address.
//
// It will block until all connections have not been established.
// There are no deadlines/timeouts.
//
// NewPool fails if any of the connections fails.
func NewPool(addrs ...string) (*Pool, error) {
	p := &Pool{
		RWMutex: &sync.RWMutex{},
		conns:   make([]*grpc.ClientConn, 0, len(addrs)),
	}

	for _, addr := range addrs {
		if err := p.Add(addr); err != nil {
			return nil, err
		}
	}

	if nbconns := len(p.conns); nbconns > 0 {
		p.current = rand.Intn(nbconns) // start round-robin at random position
	}
	return p, nil
}

// Add adds a new connection to the pool.
//
// It blocks until the connection has been successfully established.
func (p *Pool) Add(addr string) error {
	conn, err := grpc.Dial(addr,
		grpc.WithInsecure(), // no transport security
		grpc.WithBlock(),    // blocks until connection is first established
	)
	if err != nil {
		return err
	}

	p.Lock()
	p.conns = append(p.conns, conn)
	p.Unlock()

	return nil
}

// -----------------------------------------------------------------------------

// Size returns the size of the pool.
func (p *Pool) Size() int {
	p.RLock()
	defer p.RUnlock()
	return len(p.conns)
}

// ConnRoundRobin returns the next healthy connection from the pool, using a
// simple round-robin algorithm.
//
// If no healthy connection can be found, ConnRoundRobin returns `nil`.
func (p *Pool) ConnRoundRobin() (conn *grpc.ClientConn) {
	p.Lock()
	var err error
	start := p.current
	state := grpc.Idle
	for conn == nil || state != grpc.Ready {
		// find next connection in the pool
		if p.current >= len(p.conns) {
			p.current = 0
		}
		conn = p.conns[p.current]

		// fetch its state
		state, err = conn.State()
		if err != nil { // cannot fetch state, declare as not ready
			state = grpc.Idle
		}

		p.current++
		if p.current == start { // no healthy connection found, return `nil`
			conn = nil
			break
		}
	}
	p.Unlock()
	return conn
}

// Close terminates and deletes all the connections present in the pool.
//
// It returns the first error met.
func (p *Pool) Close() (err error) {
	p.Lock()
	defer p.Unlock()

	for _, c := range p.conns {
		if e := c.Close(); e != nil && err == nil {
			err = e
		}
	}

	p.conns = p.conns[0:0] // clear connection list
	return
}
