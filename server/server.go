package main

import (
	"fmt"
	"flag"
	//	"io"
	"os"
	"strings"
	"strconv"
	"bufio"
	"net"
	"crypto/md5"
	"hash"
)

const (
	kStateSetup = iota
	kStateConfig
	kStateGetMode
	kStatePutMode
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
				leanState = kStatePutMode
			} else {
				fmt.Println("Command Error, ignoring line.")
			}
		case kStateGetMode:
			for i := 0; i < len(stats.Name); i++ {
				fileInfo, error := os.Stat(stats.Name[i])
				if error != nil || !fileInfo.IsRegular() {
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
				writer.WriteString("LENGTH " + strconv.Itoa64(fileInfo.Size) + "\n")

				buffer := make([]byte, 1024)
				troll := 0
				readBytes, error := file.Read(buffer)
				checksum.Write(buffer)
				for error == nil {
					troll += readBytes
					writer.WriteString(string(buffer[:readBytes]))
					readBytes, error = file.Read(buffer)
				}
				fmt.Println("Sent", troll, "bytes from file", stats.Name[i]+".")
				writer.WriteString("CHECKSUM " + fmt.Sprintf("%x", checksum.Sum()) + "\n\n")
				writer.Flush()
			}
			state = kStateSetup
			leanState = kStateConfig
		case kStatePutMode:
			// handle file tx
			fmt.Println("PUT MODE ACTIVATED!")
			state = kStateSetup
			leanState = kStateConfig
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
