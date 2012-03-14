package main

import (
	"flag"
	"fmt"
	//	"io"
	"bufio"
	"crypto/md5"
	"hash"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	kStateSetup = iota
	kStateConfig
	kStateGetMode
	kStatePutMode
	kStatePutReceive
	kStateTeardown
)

type TxStat struct {
	Name     []string
	Length   []int64
	Checksum []string
}

var (
	Port string
)

func InitFlags() {
	flag.StringVar(&Port, "p", "65500", "Port number to listen on.")
	flag.Parse()
}

func ClientHandler(connx *net.TCPConn) {

	var stats *TxStat
	var state int = kStateSetup
	var leanState int = kStateConfig
	var rxLength int64

	temp := make([]string, 256)

	reader := bufio.NewReader(connx)
	writer := bufio.NewWriter(connx)

	defer connx.Close()

	for {
		switch state {
		case kStateSetup:
			stats = new(TxStat)
			state = kStateConfig
		case kStateConfig:
			line, prefix, error := reader.ReadLine()
			if error != nil {
				fmt.Println("Connection terminated.")
				return
			}
			temp = append(temp, string(line))
			if prefix {
				continue
			}
			toParse := strings.Join(temp, "")
			temp = make([]string, 256)

			if toParse == "BYE" {
				state = kStateTeardown
			} else if toParse == "" {
				state = leanState
			} else if strings.HasPrefix(toParse, "GET ") {
				if leanState == kStatePutMode {
					fmt.Println("Connection terminated. Attempted GET in PUT mode.")
					writer.WriteString("EPICFAIL Get & Put cannot be done in the same request\n")
					writer.Flush()

					state = kStateSetup
					leanState = kStateConfig
					continue
				}
				stats.Name = append(stats.Name, toParse[4:])
				leanState = kStateGetMode
			} else if strings.HasPrefix(toParse, "PUT ") {
				if leanState == kStateGetMode {
					fmt.Println("Connection terminated. Attempted PUT in GET mode.")
					writer.WriteString("EPICFAIL Get & Put cannot be done in the same request\n")
					writer.Flush()

					state = kStateSetup
					leanState = kStateConfig
					continue
				}
				stats.Name = append(stats.Name, toParse[4:])
				rxLength = 0
				state = kStatePutMode
				leanState = kStatePutMode
			} else {
				fmt.Println("Command Error, ignoring line.")
				state = kStateSetup
				leanState = kStateConfig
			}
		case kStateGetMode:
			for i := 0; i < len(stats.Name); i++ {
				fileInfo, error := os.Stat(stats.Name[i])
				if error != nil || fileInfo.IsDir() {
					fmt.Println("Failed to stat file " + stats.Name[i])
					writer.WriteString("NOTFOUND " + stats.Name[i] + "\n\n")
					writer.Flush()
					continue
				}

				file, error := os.Open(stats.Name[i])
				if error != nil {
					fmt.Println("Failed to open file " + stats.Name[i])
					writer.WriteString("READERR " + stats.Name[i] + "\n\n")
					writer.Flush()
					continue
				}
				defer file.Close()

				var checksum hash.Hash = md5.New()

				writer.WriteString("OK " + stats.Name[i] + "\n")
				writer.WriteString("LENGTH " + strconv.FormatInt(fileInfo.Size(), 10) + "\n\n")

				buffer := make([]byte, 1024)
				troll := 0
				readBytes, error := file.Read(buffer)
				for error == nil {
					troll += readBytes
					checksum.Write(buffer[:readBytes])
					writer.WriteString(string(buffer[:readBytes]))
					readBytes, error = file.Read(buffer)
				}
				fmt.Println("Sent", troll, "bytes from file", stats.Name[i]+".")
				writer.WriteString("\nCHECKSUM " + fmt.Sprintf("%x", checksum.Sum(make([]byte, 0))) + "\n\n")
				writer.Flush()
			}
			state = kStateSetup
			leanState = kStateConfig
		case kStatePutMode:
			line, prefix, error := reader.ReadLine()
			if error != nil {
				fmt.Println("File PUT failed.")
				writer.WriteString("FAIL " + stats.Name[0] + "\n")
				return
			}
			temp = append(temp, string(line))
			if prefix {
				continue
			}
			toParse := strings.Join(temp, "")
			temp = make([]string, 256)
			if strings.HasPrefix(toParse, "LENGTH ") {
				rxLength, error = strconv.ParseInt(toParse[7:], 10, 64)
			} else if toParse == "" && rxLength > 0 {
				state = kStatePutReceive
			}
		case kStatePutReceive:
			var count int64
			var checksum hash.Hash = md5.New()
			buffer := make([]byte, 1024)

			file, error := os.Create(stats.Name[0])
			if error != nil {
				fmt.Println("Failed to open file " + stats.Name[0])
				writer.WriteString("WRERR " + stats.Name[0] + "\n")
				writer.Flush()
				state = kStateSetup
				leanState = kStateConfig
			}
			defer file.Close()

			for count < rxLength-1024 {
				readBytes, error := reader.Read(buffer)
				if error != nil {
					fmt.Println("Stream read error.")
					writer.WriteString("RECVERR " + stats.Name[0] + "\n")
					return
				}
				count += int64(readBytes)
				checksum.Write(buffer[:readBytes])
				file.Write(buffer[:readBytes])
			}

			for count < rxLength {
				buf := make([]byte, 1)
				readBytes, error := reader.Read(buf)
				if error != nil {
					fmt.Println("Stream read error.")
					writer.WriteString("RECVERR " + stats.Name[0] + "\n")
					return
				}
				count += int64(readBytes)
				checksum.Write(buf)
				file.Write(buf)
			}

			for {
				// get checksum!
				line, prefix, error := reader.ReadLine()
				if error != nil || prefix {
					fmt.Println("Stream read error.")
					writer.WriteString("RECVERR " + stats.Name[0] + "\n")
					return
				}
				toParse := string(line)
				var csum string
				if strings.HasPrefix(toParse, "CHECKSUM ") {
					csum = toParse[9:]
				} else {
					continue
				}

				if csum != fmt.Sprintf("%x", checksum.Sum(make([]byte, 0))) {
					fmt.Println("Hash mismatch: sender claimed " + csum + ", got " + fmt.Sprintf("%x", checksum.Sum(make([]byte, 0))) + ".")
					writer.WriteString("HASHERR " + stats.Name[0] + "\n")
					writer.Flush()
					state = kStateSetup
					leanState = kStateConfig
					break
				}

				fmt.Println("Wrote " + strconv.FormatInt(count, 10) + " bytes to file " + stats.Name[0])
				writer.WriteString("RECV " + stats.Name[0] + "\n")
				writer.Flush()

				state = kStateSetup
				leanState = kStateConfig
				break
			}
		case kStateTeardown:
			fmt.Println("Connection closed by client.")
			return
		}
	}
}

func main() {
	InitFlags()

	listenPort := ":" + Port
	tcpAddress, error := net.ResolveTCPAddr("tcp", listenPort)

	if error != nil {
		fmt.Println("Resolve Error")
		return
	}

	tcpListener, error := net.ListenTCP("tcp", tcpAddress)

	if error != nil {
		fmt.Println("Listen Error")
		return
	}

	defer tcpListener.Close()

	for {
		connx, error := tcpListener.AcceptTCP()

		if error != nil {
			fmt.Println("Accept Error")
			return
		}

		go ClientHandler(connx)
	}
}
