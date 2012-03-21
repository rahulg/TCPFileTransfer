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
	kGetWaitOK = iota
	kGetWaitLength
	kGetRecvData
	kGetChecksum
	kGetDone
)

var (
	Host        string
	Port        string
	TxMode      int
	UIMutex     sync.Mutex
	NetWorkerWG sync.WaitGroup
	ServerAddr  *net.TCPAddr
)

func InitFlags() {
	flag.StringVar(&Host, "host", "127.0.0.1", "Hostname or IP address to connect to.")
	flag.StringVar(&Port, "port", "65500", "Port number to connect to.")
	flag.Parse()
}

func ParseGetResponse(filename string, reader *bufio.Reader) {

	parserState := kGetWaitOK
	temp := make([]string, 0)

	for parserState == kGetWaitOK {

		line, prefix, error := reader.ReadLine()
		if error != nil {
			fmt.Println("Connection terminated:", error)
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

		case "REQERR":

			fmt.Println("Request Error.")
			return

		case "NOTFOUND":

			if len(input) < 2 {
				fmt.Println("Connection error, invalid response format.")
			} else {
				fmt.Println("File", input[1], "was not found on the server.")
			}
			return

		case "READERR":

			if len(input) < 2 {
				fmt.Println("Connection error, invalid response format.")
			} else {
				fmt.Println("Unable to read file", input[1]+".")
			}
			return

		case "OK":

			if len(input) < 2 {
				fmt.Println("Connection error, invalid response format.")
				return
			} else if input[1] != filename {
				fmt.Println("Unexpected file", input[1]+", was expecting", filename)
				return
			} else {
				parserState = kGetWaitLength
			}
		}
	}

	var rxLength int64
	headerValid := false

	for parserState == kGetWaitLength {

		line, prefix, error := reader.ReadLine()
		if error != nil {
			fmt.Println("Connection terminated:", error)
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

		case "LENGTH":

			if len(input) < 2 {
				fmt.Println("Connection error, invalid response format.")
				return
			} else {
				rxLength, error = strconv.ParseInt(input[1], 10, 64)
				if error != nil {
					fmt.Println("Error parsing LENGTH:", error)
					break
				}
				headerValid = true
			}

		case "":

			if headerValid {
				parserState = kGetRecvData
			}

		}

	}

	var count int64
	var localFile string
	checksum := md5.New()

	for parserState == kGetRecvData {

		buffer := make([]byte, 1024)

		localFile = filename + "-part"

		file, error := os.Create(localFile)
		if error != nil {

			fmt.Println("Error creating", filename, ":", error)
			return

		}
		defer file.Close()

		for count < rxLength-1024 {

			readBytes, error := reader.Read(buffer)
			if error != nil {
				fmt.Println("Connection terminated:", error)
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
				fmt.Println("Connection terminated:", error)
				return
			}

			count += int64(readBytes)
			checksum.Write(smallBuffer[:readBytes])
			file.Write(smallBuffer[:readBytes])

		}

		parserState = kGetChecksum
	}

	for parserState == kGetChecksum {

		line, prefix, error := reader.ReadLine()
		if error != nil {
			fmt.Println("Connection terminated:", error)
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

		if input[0] == "CHECKSUM" {

			if len(input) < 2 {

				fmt.Println("Connection error, invalid response format.")
				return

			} else {

				inputChecksum = input[1]

			}

		} else {
			continue
		}

		if inputChecksum != fmt.Sprintf("%x", checksum.Sum(make([]byte, 0))) {

			fmt.Println("Hash mismatch: server claimed", inputChecksum+", received", fmt.Sprintf("%x", checksum.Sum(make([]byte, 0)))+".")
			return

		}

		parserState = kGetDone

	}

	// parserState == kGetDone
	fmt.Println("Wrote", strconv.FormatInt(count, 10), "bytes to file", filename+".")
	os.Rename(localFile, filename)

}

func GetRequest(filenames []string, pipelined bool) {

	fmt.Println("Getting", filenames, "Pipelined:", pipelined)
	connx, error := net.DialTCP("tcp", nil, ServerAddr)
	if error != nil {
		fmt.Println("Error connecting to server:", error)
		NetWorkerWG.Done()
		return
	}

	reader := bufio.NewReader(connx)
	writer := bufio.NewWriter(connx)
	defer connx.Close()

	if pipelined {

		for i := 0; i < len(filenames); i++ {
			writer.WriteString("GET " + filenames[i] + "\n")
		}

		writer.WriteString("\n")
		writer.Flush()

		for i := 0; i < len(filenames); i++ {
			ParseGetResponse(filenames[i], reader)
		}

	} else {

		for i := 0; i < len(filenames); i++ {
			writer.WriteString("GET " + filenames[i] + "\n\n")
			writer.Flush()
			ParseGetResponse(filenames[i], reader)
		}

	}

	writer.WriteString("BYE")
	writer.Flush()
	NetWorkerWG.Done()

}

func GetIndex() (filenames []string) {

	remoteFiles := make([]string, 0)

	connx, error := net.DialTCP("tcp", nil, ServerAddr)
	if error != nil {
		fmt.Println("Error connecting to server:", error)
		NetWorkerWG.Done()
		return
	}

	reader := bufio.NewReader(connx)
	writer := bufio.NewWriter(connx)
	defer connx.Close()

	writer.WriteString("GET \n\n")
	writer.Flush()

	parserState := kGetWaitOK
	temp := make([]string, 0)

	for parserState == kGetWaitOK {

		line, prefix, error := reader.ReadLine()
		if error != nil {
			fmt.Println("Connection terminated:", error)
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

		case "NOTFOUND":

			if len(input) < 2 {
				fmt.Println("Connection error, invalid response format.")
			} else {
				fmt.Println("File", input[1], "was not found on the server.")
			}
			return

		case "OK":

			if len(input) < 2 {
				fmt.Println("Connection error, invalid response format.")
				return
			} else if input[1] != "" {
				fmt.Println("Unexpected file", input[1]+", was expecting index.")
				return
			} else {
				parserState = kGetWaitLength
			}
		}
	}

	var rxLength int64
	headerValid := false

	for parserState == kGetWaitLength {

		line, prefix, error := reader.ReadLine()
		if error != nil {
			fmt.Println("Connection terminated:", error)
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

		case "LENGTH":

			if len(input) < 2 {

				fmt.Println("Connection error, invalid response format.")
				return

			} else {

				rxLength, error = strconv.ParseInt(input[1], 10, 64)
				if error != nil {
					fmt.Println("Error parsing LENGTH:", error)
					break
				}
				headerValid = true

			}

		case "":

			if headerValid {
				parserState = kGetRecvData
			}

		}

	}

	var count int64
	checksum := md5.New()
	listBuffer := make([]byte, rxLength)

	for parserState == kGetRecvData {

		buffer := make([]byte, 1024)

		for count < rxLength-1024 {

			readBytes, error := reader.Read(buffer)
			if error != nil {
				fmt.Println("Connection terminated:", error)
				return
			}

			copy(listBuffer[count:], buffer[:readBytes])
			count += int64(readBytes)
			checksum.Write(buffer[:readBytes])

		}

		smallBuffer := make([]byte, 1)

		for count < rxLength {

			readBytes, error := reader.Read(smallBuffer)
			if error != nil {
				fmt.Println("Connection terminated:", error)
				return
			}

			copy(listBuffer[count:], smallBuffer[:readBytes])
			count += int64(readBytes)
			checksum.Write(smallBuffer[:readBytes])

		}

		parserState = kGetChecksum
	}

	for parserState == kGetChecksum {

		line, prefix, error := reader.ReadLine()
		if error != nil {
			fmt.Println("Connection terminated:", error)
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

		if input[0] == "CHECKSUM" {

			if len(input) < 2 {

				fmt.Println("Connection error, invalid response format.")
				return

			} else {

				inputChecksum = input[1]

			}

		} else {
			continue
		}

		if inputChecksum != fmt.Sprintf("%x", checksum.Sum(make([]byte, 0))) {

			fmt.Println("Hash mismatch: server claimed", inputChecksum+", received", fmt.Sprintf("%x", checksum.Sum(make([]byte, 0)))+".")
			return

		}

		parserState = kGetDone

	}

	// parserState == kGetDone
	writer.WriteString("BYE\n")
	writer.Flush()
	tempString := string(listBuffer)
	remoteFiles = strings.Split(tempString, "\n")

	return remoteFiles

}

func GetFiles(filenames []string) {

	switch TxMode {

	case kTXModeSingle:

		for i := 0; i < len(filenames); i++ {

			NetWorkerWG.Add(1)
			temp := make([]string, 1)
			temp[0] = filenames[i]
			go GetRequest(temp, false)
			NetWorkerWG.Wait()

		}

	case kTXModeParallel:

		for i := 0; i < len(filenames); i++ {

			NetWorkerWG.Add(1)
			temp := make([]string, 1)
			temp[0] = filenames[i]
			go GetRequest(temp, false)

		}

		NetWorkerWG.Wait()

	case kTXModePersistent, kTXModePipelined:

		NetWorkerWG.Add(1)
		go GetRequest(filenames, TxMode == kTXModePipelined)
		NetWorkerWG.Wait()

	}

	UIMutex.Unlock()

}

func GetAll() {

	fileList := GetIndex()
	GetFiles(fileList[:len(fileList)-1])

}

func PutRequestSend(filename string, writer *bufio.Writer) {

	fileInfo, error := os.Stat(filename)
	if error != nil || fileInfo.IsDir() {
		fmt.Println("File", filename, "not found.")
		return
	}

	file, error := os.Open(filename)
	if error != nil {
		fmt.Println("Could not open", filename+".")
		return
	}
	defer file.Close()

	checksum := md5.New()

	writer.WriteString("PUT " + filename + "\n")
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

	writer.WriteString("\n\nCHECKSUM " + fmt.Sprintf("%x", checksum.Sum(make([]byte, 0))) + "\n\n")
	writer.Flush()

}

func PutRequest(filenames []string, pipelined bool) {

	fmt.Println("Putting", filenames, "Pipelined:", pipelined)
	connx, error := net.DialTCP("tcp", nil, ServerAddr)
	if error != nil {
		fmt.Println("Error connecting to server:", error)
		NetWorkerWG.Done()
		return
	}

	reader := bufio.NewReader(connx)
	writer := bufio.NewWriter(connx)
	defer connx.Close()

	if pipelined {

		for i := 0; i < len(filenames); i++ {
			PutRequestSend(filenames[i], writer)
		}

		for i := 0; i < len(filenames); i++ {

			line, _, error := reader.ReadLine()
			if error != nil {
				fmt.Println("Connection terminated:", error)
				return
			}

			toParse := string(line)
			input := strings.Split(toParse, " ")

			if input[0] == "RECV" {
				fmt.Println("Sent file", input[1]+".")
			} else if input[1] == "WRERR" {
				fmt.Println("Failed to write file", input[1]+".")
			}

		}

	} else {

		for i := 0; i < len(filenames); i++ {

			PutRequestSend(filenames[i], writer)

			line, _, error := reader.ReadLine()
			if error != nil {
				fmt.Println("Connection terminated:", error)
				return
			}

			toParse := string(line)
			input := strings.Split(toParse, " ")

			if input[0] == "RECV" {
				fmt.Println("Sent file", input[1]+".")
			} else if input[1] == "WRERR" {
				fmt.Println("Failed to write file", input[1]+".")
			}

		}

	}

	writer.WriteString("BYE")
	writer.Flush()
	NetWorkerWG.Done()

}

func PutFiles(filenames []string) {

	switch TxMode {
	case kTXModeSingle:
		for i := 0; i < len(filenames); i++ {
			NetWorkerWG.Add(1)
			temp := make([]string, 1)
			temp[0] = filenames[i]
			go PutRequest(temp, false)
			NetWorkerWG.Wait()
		}
	case kTXModeParallel:
		for i := 0; i < len(filenames); i++ {
			NetWorkerWG.Add(1)
			temp := make([]string, 1)
			temp[0] = filenames[i]
			go PutRequest(temp, false)
		}
		NetWorkerWG.Wait()
	case kTXModePersistent, kTXModePipelined:
		NetWorkerWG.Add(1)
		go PutRequest(filenames, TxMode == kTXModePipelined)
		NetWorkerWG.Wait()
	}

	UIMutex.Unlock()

}

func main() {

	InitFlags()

	listenPort := Host + ":" + Port
	tcpAddress, error := net.ResolveTCPAddr("tcp", listenPort)
	if error != nil {
		fmt.Println("Address resolution error:", error)
		return
	}
	ServerAddr = tcpAddress

	TxMode = kTXModeSingle
	temp := make([]string, 256)

	stdin := bufio.NewReader(os.Stdin)

	for {

		UIMutex.Lock()

		fmt.Print("] ")

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

			UIMutex.Unlock()

		case "ls":
			localFilesInfo, error := ioutil.ReadDir(".")
			if error != nil {
				fmt.Println("LS failed:", error)
				UIMutex.Unlock()
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

			UIMutex.Unlock()

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
				UIMutex.Unlock()
				continue
			}

			go GetFiles(wantedFiles)

		case "rls":

			fileIndex := GetIndex()
			for i := 0; i < len(fileIndex); i++ {
				fmt.Println(fileIndex[i])
			}

			UIMutex.Unlock()

		case "getall":

			go GetAll()

		case "put":

			destFiles := make([]string, 0)

			epicfail := true
			for i := 1; i < len(input); i++ {
				if input[i] != "" {
					epicfail = false
					destFiles = append(destFiles, input[i])
				}
			}

			if epicfail {
				fmt.Println("Invalid syntax. Usage: put <file name>")
				UIMutex.Unlock()
				continue
			}

			go PutFiles(destFiles)

		default:
			fmt.Println("Unrecognised command.")
			fallthrough
		case "help":
			if len(input) < 2 {
				fmt.Println("Commands: get, getall, put, ls, rls, help, mode, quit, exit\n")
				fmt.Println("For more info type: help <command name>")
			} else {

				switch input[1] {
				case "get":
					fmt.Println("Downloads specified file(s) from the server.\n")
					fmt.Println("Usage: get <file1> [file2] [file3] …")
				case "getall":
					fmt.Println("Downloads the file index from the server and all listed files.\n")
					fmt.Println("Usage: getall")
				case "put":
					fmt.Println("Uploads the specified file(s) to the server.\n")
					fmt.Println("Usage: put <file1> [file2] [file3] …")
				case "ls":
					fmt.Println("Lists all files in the current working directory.\n")
					fmt.Println("Usage: ls")
				case "rls":
					fmt.Println("Lists all files on the server.\n")
					fmt.Println("Usage: rls")
				case "help":
					fmt.Println("If you need help for help, you need help.")
					fmt.Println("Yo dawg, I heard you like help. So I put some help in your help so you can help while you help.\n")
					fmt.Println("Usage: help                 || Lists all available commands.")
					fmt.Println("       help <command name>  || Prints info and usage for specified command.")
				case "mode":
					fmt.Println("Switches transfer modes.\n")
					fmt.Println("Usage: mode        || Prints current mode.")
					fmt.Println("       mode list   || Lists all available modes.")
					fmt.Println("       mode [mode] || Switches transfer modes to the specified mode.")
				case "quit", "exit":
					fmt.Println("Exits the application.\n")
					fmt.Println("Usage: quit")
					fmt.Println("       exit")
				default:
					fmt.Println("Commands: get, getall, put, ls, rls, help, mode, quit, exit\n")
					fmt.Println("For more info type: help <command name>")
				}

			}
			
			UIMutex.Unlock()

		case "quit", "exit":
			return
		}
	}

	return
}
