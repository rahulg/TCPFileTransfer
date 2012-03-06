package main

import (
	"fmt"
	"flag"
	//	"io"
	"strings"
	"bufio"
	"net"
)

const (
	kStateSetup = iota
	kStateConfig
	kStateGetMode
	kStatePutMode
	kStateTeardown
)

type TxStat struct {
	Name      []string
	Length    []int64
	Checksum  []string
	State     int
	LeanState int
}

var (
	Port string
)

func InitFlags() {
	flag.StringVar(&Port, "p", "65500", "Port number to listen on")
	flag.Parse()
}

func ClientHandler(connx *net.TCPConn) {

	var stats *TxStat
	stats = new(TxStat)
	stats.State = kStateSetup
	stats.LeanState = kStateConfig

	temp := make([]string, 256)

	reader := bufio.NewReader(connx)
	writer := bufio.NewWriter(connx)

	defer connx.Close()

	for {
		if stats.State == kStateSetup {
			stats = new(TxStat)
			stats.State = kStateConfig
			stats.LeanState = kStateConfig
		} else if stats.State == kStateConfig {
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

			if toParse == "EOT" {
				stats.State = kStateTeardown
			} else if toParse == "" {
				stats.State = stats.LeanState
			} else if strings.HasPrefix(toParse, "GET ") {
				if stats.LeanState == kStatePutMode {
					fmt.Println("Connection terminated. Attempted GET in PUT mode.")
					return
				}
				stats.Name = append(stats.Name, toParse[4:])
				stats.LeanState = kStateGetMode
			} else if strings.HasPrefix(toParse, "PUT ") {
				if stats.LeanState == kStateGetMode {
					fmt.Println("Connection terminated. Attempted PUT in GET mode.")
					return
				}
				stats.Name = append(stats.Name, toParse[4:])
				stats.LeanState = kStatePutMode
			} else {
				fmt.Println("Command Error, ignoring line.")
			}
		} else if stats.State == kStateGetMode {
			for i := 0; i < len(stats.Name); i++ {
				writer.WriteString("OK " + stats.Name[i] + "\n")
				writer.WriteString("LENGTH troll\n")
				// send file
				writer.WriteString("Here is your file.\n")
				writer.WriteString("CHECKSUM troll\n\n")
				writer.Flush()
			}
			stats.State = kStateSetup
		} else if stats.State == kStatePutMode {
			// handle file tx
			fmt.Println("PUT MODE ACTIVATED!")
		} else if stats.State == kStateTeardown {
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
