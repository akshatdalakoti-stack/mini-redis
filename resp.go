package main

import (
	"bufio"
	"errors"
	"strconv"
	"strings"
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
		return strings.Fields(line), nil
	}

	count, err := strconv.Atoi(line[1:])
	if err != nil {
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

		bulkLen, err := strconv.Atoi(bulkHeader[1:])
		if err != nil {
			return nil, errors.New("invalid bulk length")
		}

		data := make([]byte, bulkLen+2) // +2 for the trailing \r\n
		_, err = readFull(reader, data)
		if err != nil {
			return nil, err
		}

		args = append(args, string(data[:bulkLen]))
	}

	return args, nil
}

// readLine reads until \r\n and strips it off
func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	return line, nil
}

func readFull(reader *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := reader.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// everything below here writes RESP replies back to the client

func writeSimpleString(writer *bufio.Writer, s string) {
	writer.WriteString("+" + s + "\r\n")
	writer.Flush()
}

func writeError(writer *bufio.Writer, s string) {
	writer.WriteString("-" + s + "\r\n")
	writer.Flush()
}

func writeInteger(writer *bufio.Writer, n int) {
	writer.WriteString(":" + strconv.Itoa(n) + "\r\n")
	writer.Flush()
}

func writeBulkString(writer *bufio.Writer, s string) {
	writer.WriteString("$" + strconv.Itoa(len(s)) + "\r\n")
	writer.WriteString(s + "\r\n")
	writer.Flush()
}

func writeNullBulkString(writer *bufio.Writer) {
	writer.WriteString("$-1\r\n")
	writer.Flush()
}

func writeArray(writer *bufio.Writer, items []string) {
	writer.WriteString("*" + strconv.Itoa(len(items)) + "\r\n")
	for _, item := range items {
		writer.WriteString("$" + strconv.Itoa(len(item)) + "\r\n")
		writer.WriteString(item + "\r\n")
	}
	writer.Flush()
}

func writeEmptyArray(writer *bufio.Writer) {
	writer.WriteString("*0\r\n")
	writer.Flush()
}
