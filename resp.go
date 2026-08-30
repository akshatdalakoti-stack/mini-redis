package main

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"strings"
)

// maxArrayLen / maxBulkLen cap the sizes we're willing to allocate off a
// single length header. A broken or malicious client sending "*99999999"
// or "$99999999" shouldn't be able to make us reserve gigabytes before a
// single byte of payload has arrived.
const (
	maxArrayLen = 1 << 20
	maxBulkLen  = 512 * 1024 * 1024
)

// readCommand reads one RESP command from the client and returns it as a
// slice of strings, e.g. SET foo bar becomes ["SET", "foo", "bar"].
// redis-cli always sends commands as RESP arrays of bulk strings, so that
// is the main format we handle. We also handle plain text lines (inline
// commands) so you can test the server with something like telnet or nc.
func readCommand(reader *bufio.Reader) ([]string, error) {
	line, err := readLine(reader)
	if err != nil {
		return nil, err
	}

	if len(line) == 0 {
		return []string{}, nil
	}

	if line[0] != '*' {
		// inline command: rare path (telnet/nc), so the string copy that
		// strings.Fields needs is not worth optimising away.
		return strings.Fields(string(line)), nil
	}

	count, err := atoiBytes(line[1:])
	if err != nil || count < 0 || count > maxArrayLen {
		return nil, errors.New("invalid array length")
	}

	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		bulkHeader, err := readLine(reader)
		if err != nil {
			return nil, err
		}
		if len(bulkHeader) == 0 || bulkHeader[0] != '$' {
			return nil, errors.New("expected bulk string")
		}

		bulkLen, err := atoiBytes(bulkHeader[1:])
		if err != nil || bulkLen < 0 || bulkLen > maxBulkLen {
			return nil, errors.New("invalid bulk length")
		}

		// allocate exactly the payload size (no +2 slack) and read it in
		// one shot, then skip the trailing \r\n without copying it.
		data := make([]byte, bulkLen)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
		if _, err := reader.Discard(2); err != nil {
			return nil, err
		}

		args = append(args, string(data))
	}

	return args, nil
}

// readLine reads one \r\n-terminated line and returns it without the
// terminator. The result aliases the reader's internal buffer and is only
// valid until the next read from reader, so callers must copy anything they
// need to keep. Using ReadSlice here avoids the two allocations that
// ReadString + strings.TrimRight cost on every header line.
func readLine(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadSlice('\n')
	if err == nil {
		return trimCRLF(line), nil
	}
	if errors.Is(err, bufio.ErrBufferFull) {
		// Line longer than the buffer, e.g. a giant inline command. Fall
		// back to the allocating path; RESP headers never hit this.
		buf := append([]byte(nil), line...)
		rest, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		return trimCRLF(append(buf, rest...)), nil
	}
	return nil, err
}

func trimCRLF(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\r' || b[len(b)-1] == '\n') {
		b = b[:len(b)-1]
	}
	return b
}

// atoiBytes parses a non-negative base-10 integer straight out of b,
// skipping the string allocation that strconv.Atoi(string(b)) would incur
// on every array and bulk header.
func atoiBytes(b []byte) (int, error) {
	if len(b) == 0 || len(b) > 18 { // 18 digits stays well inside int64
		return 0, errors.New("invalid integer")
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, errors.New("invalid integer")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// everything below here writes RESP replies back to the client.
//
// none of these flush: handleConnection flushes once per read batch, so a
// pipeline of N commands turns into a single write() syscall instead of N.
// building each reply with separate WriteString/WriteByte calls also avoids
// the throwaway string that "+" + s + "\r\n" would allocate on every reply.

const crlf = "\r\n"

func writeCRLF(writer *bufio.Writer) {
	writer.WriteString(crlf)
}

func writeSimpleString(writer *bufio.Writer, s string) {
	writer.WriteByte('+')
	writer.WriteString(s)
	writeCRLF(writer)
}

func writeError(writer *bufio.Writer, s string) {
	writer.WriteByte('-')
	writer.WriteString(s)
	writeCRLF(writer)
}

func writeInteger(writer *bufio.Writer, n int) {
	writer.WriteByte(':')
	writeInt(writer, n)
	writeCRLF(writer)
}

func writeBulkString(writer *bufio.Writer, s string) {
	writer.WriteByte('$')
	writeInt(writer, len(s))
	writeCRLF(writer)
	writer.WriteString(s)
	writeCRLF(writer)
}

func writeNullBulkString(writer *bufio.Writer) {
	writer.WriteString("$-1\r\n")
}

func writeArray(writer *bufio.Writer, items []string) {
	writer.WriteByte('*')
	writeInt(writer, len(items))
	writeCRLF(writer)
	for _, item := range items {
		writeBulkString(writer, item)
	}
}

func writeEmptyArray(writer *bufio.Writer) {
	writer.WriteString("*0\r\n")
}

// writeInt writes n in base 10 without allocating a string (strconv.Itoa
// would). The scratch array lives on the stack.
func writeInt(writer *bufio.Writer, n int) {
	var buf [20]byte
	writer.Write(strconv.AppendInt(buf[:0], int64(n), 10))
}
