package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	//	"crypto/md5"
	//	"hash"
	"net"
	"os"
	//"strconv"
	"strings"
	"sync"
)

const (
	kTXModeSingle = iota
	kTXModeParallel
	kTXModePersistent
	kTXModePipelined
)

var TXModeStrings = [4]string{"single", "parallel", "persistent", "pipelined"}

const (
	kStateSetup = iota
	kStateGetMode
	kStateGetReceive
	kStatePutMode
	kStateTeardown
)

var (
	Host   string
	Port   string
	TxMode int
	IOMutex    sync.Mutex
)

func InitFlags() {
	flag.StringVar(&Host, "host", "127.0.0.1", "Hostname or IP address to connect to.")
	flag.StringVar(&Port, "port", "65500", "Port number to connect to.")
	flag.Parse()
}

func GetAll() {
	fmt.Println("GetAll")
	IOMutex.Unlock()
}

func GetFiles(filenames []string) {
	fmt.Println("Getting: ", filenames)
	IOMutex.Unlock()
}

func PutFile(filename string) {
	fmt.Println("Putting: ", filename)
	IOMutex.Unlock()
}

func main() {

	InitFlags()

	listenPort := Host + ":" + Port
	tcpAddress, error := net.ResolveTCPAddr("tcp", listenPort)
	if error != nil {
		fmt.Println("Address resolution error:", error)
		return
	}
	tcpAddress = tcpAddress

	TxMode = kTXModeSingle
	temp := make([]string, 256)

	stdin := bufio.NewReader(os.Stdin)

	for {

		IOMutex.Lock()

		fmt.Print("> ")

		line, prefix, error := stdin.ReadLine()
		if error != nil {
			fmt.Println("")
			return
		}
		temp = append(temp, string(line))
		if prefix {
			continue
		}

		toParse := strings.Join(temp, "")
		input := strings.Split(toParse, " ")
		temp = make([]string, 256)

		switch input[0] {
		case "mode":

			if len(input) == 1 {

				fmt.Println("Current Mode:", TXModeStrings[TxMode])

			} else if len(input) == 2 {

				switch input[1] {

				case "single":
					fmt.Println("Mode:", TXModeStrings[TxMode], "=> single")
					TxMode = kTXModeSingle

				case "parallel":
					fmt.Println("Mode:", TXModeStrings[TxMode], "=> parallel")
					TxMode = kTXModeParallel

				case "persistent":
					fmt.Println("Mode:", TXModeStrings[TxMode], "=> persistent")
					TxMode = kTXModePersistent

				case "pipelined":
					fmt.Println("Mode:", TXModeStrings[TxMode], "=> pipelined")
					TxMode = kTXModePipelined

				case "list":
					fallthrough
				default:
					fmt.Print("Available modes:")
					for i := 0; i < len(TXModeStrings); i++ {
						fmt.Print(" " + TXModeStrings[i])
					}
					fmt.Println("")
				}

			} else {
				fmt.Println("Invalid syntax. Usage: mode <single/parallel/persistent/pipelined>")
			}

			IOMutex.Unlock()

		case "ls":
			localFilesInfo, error := ioutil.ReadDir(".")
			if error != nil {
				fmt.Println("LS failed:", error)
				IOMutex.Unlock()
				continue
			}

			localFileNames := make([]string, 0)
			localFileSizes := make([]int64, 0)
			maxNameSize := 0

			for i := 0; i < len(localFilesInfo); i++ {
				theFileName := localFilesInfo[i].Name()
				theFileSize := localFilesInfo[i].Size()
				if len(theFileName) > maxNameSize {
					maxNameSize = len(theFileName)
				}

				localFileNames = append(localFileNames, theFileName)
				localFileSizes = append(localFileSizes, theFileSize)
			}

			for i := 0; i < len(localFileNames); i++ {
				if !strings.HasPrefix(localFileNames[i], ".") {

					fmt.Print(localFileNames[i])
					for j := len(localFileNames[i]); j < maxNameSize; j++ {
						fmt.Print(" ")
					}

					if localFileSizes[i] > 1048576 {
						fmt.Println("\t", localFileSizes[i]/1048576, "MB")
					} else if localFileSizes[i] > 1024 {
						fmt.Println("\t", localFileSizes[i]/1024, "kB")
					} else {
						fmt.Println("\t", localFileSizes[i], "B")
					}

				}
			}

			IOMutex.Unlock()

		case "get":

			wantedFiles := make([]string, 0)

			epicfail := true
			for i := 1; i < len(input); i++ {
				if input[i] != "" {
					epicfail = false
					wantedFiles = append(wantedFiles, input[i])
				}
			}

			if epicfail {
				fmt.Println("Invalid syntax. Usage: get <file1> [file2] …")
				IOMutex.Unlock()
				continue
			}

			go GetFiles(wantedFiles)

		case "getall":

			go GetAll()

		case "put":

			var destFile string

			epicfail := true
			for i := 1; i < len(input); i++ {
				if input[i] != "" {
					epicfail = false
					destFile = input[i]
					break
				}
			}

			if epicfail {
				fmt.Println("Invalid syntax. Usage: put <file name>")
				IOMutex.Unlock()
				continue
			}

			go PutFile(destFile)

		default:
			fmt.Println("Unrecognised command.")
			fallthrough
		case "help":
			fmt.Println("Commands: get, getall, put, ls, help, mode, quit, exit")
			IOMutex.Unlock()
			
		case "quit", "exit":
			return
		}
	}

	return
}
