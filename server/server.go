package main

import (
	"bufio"
	"crypto/md5"
	"flag"
	"fmt"
	"io/ioutil"
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

var (
	Port string
)

func InitFlags() {
	flag.StringVar(&Port, "port", "65500", "Port number to listen on.")
	flag.Parse()
}

func ClientHandler(connx *net.TCPConn) {

	var filenames []string
	var state int = kStateSetup
	var leanState int = kStateConfig
	var rxLength int64

	temp := make([]string, 0)

	reader := bufio.NewReader(connx)
	writer := bufio.NewWriter(connx)

	defer connx.Close()

	for {
		switch state {

		case kStateSetup:

			filenames = append(make([]string, 0))
			state = kStateConfig

		case kStateConfig:

			line, prefix, error := reader.ReadLine()
			if error != nil {
				fmt.Println("[", connx.RemoteAddr(), "] Connection terminated:", error)
				return
			}
			temp = append(temp, string(line))
			if prefix {
				continue
			}

			toParse := strings.Join(temp, "")
			input := strings.Split(toParse, " ")
			temp = make([]string, 0)

			switch input[0] {

			case "BYE":

				state = kStateTeardown

			case "":

				state = leanState

			case "GET":

				if leanState == kStatePutMode || len(input) < 2 {

					fmt.Println("[", connx.RemoteAddr(), "] Request Format Error:", input)
					writer.WriteString("REQERR\n")
					writer.Flush()

					state = kStateSetup
					leanState = kStateConfig
					continue

				}

				filename := input[1]

				// Pseudo-chroot jail
				touched := true
				for touched {

					touched = false

					if len(filename) > 1 && filename[:1] == "/" {
						filename = filename[1:]
						touched = true
					}

					if len(filename) > 2 && filename[:2] == "./" {
						filename = filename[2:]
						touched = true
					}

					if len(filename) > 3 && filename[:3] == "../" {
						filename = filename[3:]
						touched = true
					}

				}

				filenames = append(filenames, filename)
				leanState = kStateGetMode

			case "PUT":

				if leanState == kStateGetMode || len(input) < 2 {

					fmt.Println("[", connx.RemoteAddr(), "] Request Format Error:", input)
					writer.WriteString("REQERR\n")
					writer.Flush()

					state = kStateSetup
					leanState = kStateConfig
					continue

				}

				filename := input[1]

				// Pseudo-chroot jail
				touched := true
				for touched {

					touched = false

					if len(filename) > 1 && filename[:1] == "/" {
						filename = filename[1:]
						touched = true
					}

					if len(filename) > 2 && filename[:2] == "./" {
						filename = filename[2:]
						touched = true
					}

					if len(filename) > 3 && filename[:3] == "../" {
						filename = filename[3:]
						touched = true
					}

				}

				filenames = append(make([]string, 0), filename)

				rxLength = 0
				state = kStatePutMode
				leanState = kStatePutMode

			default:

				fmt.Println("[", connx.RemoteAddr(), "] Unrecognised command:", input)
				state = kStateSetup
				leanState = kStateConfig

			}

		case kStateGetMode:

			for i := 0; i < len(filenames); i++ {

				if filenames[i] == "filelist.txt" || filenames[i] == "" {

					localFiles := make([]string, 0)
					localFilesInfo, error := ioutil.ReadDir("files")
					if error != nil {
						fmt.Println("[", connx.RemoteAddr(), "] Directory listing error:", error)
						writer.WriteString("NOTFOUND " + filenames[i] + "\n\n")
						continue
					}

					for i := 0; i < len(localFilesInfo); i++ {
						theFileName := localFilesInfo[i].Name()
						if !strings.HasPrefix(theFileName, ".") {
							localFiles = append(localFiles, theFileName)
						}
					}

					var totalSize int64 = 0
					for i := 0; i < len(localFiles); i++ {
						totalSize += int64(len(localFiles[i]) + 1)
					}

					checksum := md5.New()

					writer.WriteString("OK " + filenames[i] + "\n")
					writer.WriteString("LENGTH " + strconv.FormatInt(totalSize, 10) + "\n\n")

					for i := 0; i < len(localFiles); i++ {
						checksum.Write([]byte(localFiles[i] + "\n"))
						writer.WriteString(localFiles[i] + "\n")
					}

					fmt.Println("[", connx.RemoteAddr(), "] Sent", totalSize, "bytes for index")
					writer.WriteString("\n\nCHECKSUM " + fmt.Sprintf("%x", checksum.Sum(make([]byte, 0))) + "\n\n")
					writer.Flush()

				} else {

					localFile := "files/" + filenames[i]

					fileInfo, error := os.Stat(localFile)
					if error != nil || fileInfo.IsDir() {
						fmt.Println("[", connx.RemoteAddr(), "] Error stat-ing", localFile, ":", error)
						writer.WriteString("NOTFOUND " + filenames[i] + "\n\n")
						writer.Flush()
						continue
					}

					file, error := os.Open(localFile)
					if error != nil {
						fmt.Println("[", connx.RemoteAddr(), "] Error opening", localFile, ":", error)
						writer.WriteString("READERR " + filenames[i] + "\n\n")
						writer.Flush()
						continue
					}

					checksum := md5.New()

					writer.WriteString("OK " + filenames[i] + "\n")
					writer.WriteString("LENGTH " + strconv.FormatInt(fileInfo.Size(), 10) + "\n\n")

					buffer := make([]byte, 1024)
					var sentBytes int64 = 0
					readBytes, error := file.Read(buffer)
					for error == nil {

						sentBytes += int64(readBytes)

						checksum.Write(buffer[:readBytes])
						writer.WriteString(string(buffer[:readBytes]))

						readBytes, error = file.Read(buffer)

					}

					fmt.Println("[", connx.RemoteAddr(), "] Sent", sentBytes, "bytes from file", localFile+".")
					writer.WriteString("\n\nCHECKSUM " + fmt.Sprintf("%x", checksum.Sum(make([]byte, 0))) + "\n\n")
					file.Close()
					writer.Flush()

				}

			}

			state = kStateSetup
			leanState = kStateConfig

		case kStatePutMode:

			line, prefix, error := reader.ReadLine()
			if error != nil {
				fmt.Println("[", connx.RemoteAddr(), "] Connection terminated:", error)
				return
			}

			temp = append(temp, string(line))
			if prefix {
				continue
			}

			toParse := strings.Join(temp, "")
			input := strings.Split(toParse, " ")
			temp = make([]string, 0)

			if input[0] == "LENGTH" {
				if len(input) < 2 {
					fmt.Println("[", connx.RemoteAddr(), "] Error in header field \"LENGTH\"")
					continue
				}
				rxLength, error = strconv.ParseInt(input[1], 10, 64)
				if error != nil {
					fmt.Println("[", connx.RemoteAddr(), "] Error parsing header field LENGTH:", error)
					continue
				}
			} else if input[0] == "" && rxLength > 0 {
				state = kStatePutReceive
			}

		case kStatePutReceive:

			var count int64
			checksum := md5.New()
			buffer := make([]byte, 1024)

			localFile := "files/" + filenames[0] + "-part"

			file, error := os.Create(localFile)
			if error != nil {

				fmt.Println("[", connx.RemoteAddr(), "] Error creating", localFile, ":", error)
				writer.WriteString("WRERR " + filenames[0] + "\n\n")
				writer.Flush()

				state = kStateSetup
				leanState = kStateConfig
				continue

			}
			defer file.Close()

			for count < rxLength-1024 {

				readBytes, error := reader.Read(buffer)
				if error != nil {
					fmt.Println("[", connx.RemoteAddr(), "] Connection terminated:", error)
					return
				}
				count += int64(readBytes)
				checksum.Write(buffer[:readBytes])
				file.Write(buffer[:readBytes])

			}

			smallBuffer := make([]byte, 1)

			for count < rxLength {
				readBytes, error := reader.Read(smallBuffer)
				if error != nil {
					fmt.Println("[", connx.RemoteAddr(), "] Connection terminated:", error)
					return
				}
				count += int64(readBytes)
				checksum.Write(smallBuffer[:readBytes])
				file.Write(smallBuffer[:readBytes])
			}

			temp := make([]string, 0)

			for {

				line, prefix, error := reader.ReadLine()
				if error != nil {
					fmt.Println("[", connx.RemoteAddr(), "] Connection terminated:", error)
					return
				}

				temp = append(temp, string(line))
				if prefix {
					continue
				}

				toParse := strings.Join(temp, "")
				input := strings.Split(toParse, " ")
				temp = make([]string, 0)

				var inputChecksum string
				if input[0] == "CHECKSUM" && len(input) > 1 {
					inputChecksum = input[1]
				} else {
					continue
				}

				if inputChecksum != fmt.Sprintf("%x", checksum.Sum(make([]byte, 0))) {

					fmt.Println("[", connx.RemoteAddr(), "] Hash mismatch. Sender claimed", inputChecksum+", received", fmt.Sprintf("%x", checksum.Sum(make([]byte, 0))))
					writer.WriteString("HASHERR " + filenames[0] + "\n\n")
					writer.Flush()

					state = kStateSetup
					leanState = kStateConfig
					break

				}

				os.Rename(localFile, localFile[:len(localFile)-5])
				fmt.Println("[", connx.RemoteAddr(), "] Wrote", strconv.FormatInt(count, 10), "bytes to file", localFile[:len(localFile)-5])
				writer.WriteString("RECV " + filenames[0] + "\n\n")
				writer.Flush()

				state = kStateSetup
				leanState = kStateConfig
				break

			}

		case kStateTeardown:

			fmt.Println("[", connx.RemoteAddr(), "] Connection closed by client")
			return

		}
	}
}

func main() {

	InitFlags()

	listenPort := ":" + Port
	tcpAddress, error := net.ResolveTCPAddr("tcp", listenPort)
	if error != nil {
		fmt.Println("Error resolving listening address:", error)
		return
	}

	tcpListener, error := net.ListenTCP("tcp", tcpAddress)
	if error != nil {
		fmt.Println("Error while attempting to listen:", error)
		return
	}
	defer tcpListener.Close()

	for {

		connx, error := tcpListener.AcceptTCP()
		if error != nil {
			fmt.Println("Error while accepting connection:", error)
			continue
		}

		go ClientHandler(connx)
	}
}
