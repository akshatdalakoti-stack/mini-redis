package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"sync"
)

// bumped these up from the bufio defaults (4096) since redis commands can
// come in bursts and bigger buffers = less syscalls = faster
const (
	readBufSize  = 16 * 1024
	writeBufSize = 16 * 1024
)

// reusing the reader/writer structs instead of making new ones every time
// a client connects, saves on garbage collector having to clean up so much
var readerPool = sync.Pool{
	New: func() any {
		return bufio.NewReaderSize(nil, readBufSize)
	},
}

var writerPool = sync.Pool{
	New: func() any {
		return bufio.NewWriterSize(nil, writeBufSize)
	},
}

// RunServer opens the TCP listener and accepts clients until ctx is
// cancelled. Each client gets its own goroutine so multiple redis-cli
// sessions can talk to the server at the same time.
func RunServer(ctx context.Context, store *Store, address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Println("mini-redis listening on", address)

	var wg sync.WaitGroup

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			default:
			}

			// A temporary network error (e.g. too many open files)
			// shouldn't take the whole server down; log it and keep
			// accepting new connections.
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				log.Println("accept error (temporary):", err)
				continue
			}
			return err
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			handleConnection(store, conn)
		}()
	}
}

func handleConnection(store *Store, conn net.Conn) {
	remote := conn.RemoteAddr()
	log.Println("client connected:", remote)
	defer func() {
		conn.Close()
		if r := recover(); r != nil {
			log.Println("client", remote, "crashed the handler, recovered:", r)
			return
		}
		log.Println("client disconnected:", remote)
	}()

	// turn off nagle's algorithm so single commands go out right away
	// instead of waiting around to see if more data is coming
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
	}

	// grab a reader/writer from the pool instead of allocating new ones,
	// then give them back when this client leaves
	reader := readerPool.Get().(*bufio.Reader)
	reader.Reset(conn)
	defer readerPool.Put(reader)

	writer := writerPool.Get().(*bufio.Writer)
	writer.Reset(conn)
	defer writerPool.Put(writer)

	for {
		args, err := readCommand(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Println("client", remote, "read error:", err)
			}
			return
		}
		if len(args) != 0 {
			handleCommand(store, writer, args)
		}

		// only flush once we've drained everything the client already sent.
		// for a normal request/reply client that's every command; for a
		// pipelined client it collapses a whole batch of replies into one
		// write syscall.
		if reader.Buffered() == 0 {
			if err := writer.Flush(); err != nil {
				log.Println("client", remote, "write error:", err)
				return
			}
		}
	}
}
