//go:build !darwin && !android

package ble

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	//"os"
	"strconv"

	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/pkg/term"
	"go.uber.org/zap"
	//"github.com/jacobsa/go-serial/serial"
)

// ProtocolID is the protocol ID of the libble bridge
var ProtocolID = protocol.ID("/libble-bridge/0.0.1")

// ensure term.Term always satisfies this interface
var _ Serial = (*term.Term)(nil)

// Serial is an interface to provide easy testing of the ble protocol
type Serial interface {
	Write([]byte) (int, error)
	Available() (int, error)
	Read([]byte) (int, error)
	Flush() error
	Close() error
}

// Bridge allows authorized peers to open a stream
// and read/write data through the ble bridge
type Bridge struct {
	serial          io.ReadWriteCloser
	ctx             context.Context
	wg              *sync.WaitGroup
	authorizedPeers map[peer.ID]bool
	readChan        chan []byte
	writeChan       chan []byte
	logger          *zap.Logger
}

// Opts allows configuring the bridge
type Opts struct {
	AuthorizedPeers map[peer.ID]bool // empty means allow all
}

// NewBridge returns an initialized bridge, suitable for use a LibP2P protocol
func NewBridge(ctx context.Context, wg *sync.WaitGroup, logger *zap.Logger, serial io.ReadWriteCloser, opt Opts) (*Bridge, error) {
	bridge := &Bridge{
		serial:          serial,
		ctx:             ctx,
		authorizedPeers: opt.AuthorizedPeers,
		logger:          logger.Named("ble.bridge"),
		readChan:        make(chan []byte, 1000),
		writeChan:       make(chan []byte, 1000),
		wg:              wg,
	}
	bridge.serialDumper()
	return bridge, nil
}

var data_parse []byte
var data_parse2 []byte
var dataParseFin []byte

var continue_data bool = false
var continue_sender bool = true
var waiting_ack bool = false
var continue_pid bool = false

var message_full string = ""
var message_full_old string = ""
var message_parse_old_list []string = []string{}
var message_parse_old string = ""

var sender_user string = ""
var receiver_user string = ""
var remote_pid string = ""
var pid_full string = ""

// serialDumper allows any number of libp2p streams to write
// into the ble bridge, or read from it. Reads are sent to any
// active streams
func (b *Bridge) serialDumper() {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()

		s := bufio.NewReader(b.serial)

		for {
			select {
			case <-b.ctx.Done():
				fmt.Println("DONEEEEEEEE")
				return
			default:
				data, err := s.ReadBytes('\n')

				//print("RECEIVE: ", string(data))

				if err != nil && err != io.EOF {
					b.logger.Error("error reading serial data", zap.Error(err))
					return
				}

				if len(sender_user) <= 0 || continue_sender {

					sender_user, continue_sender = parse_message(data, `\<.*\>`, `\<.*`, `.*\>`)

					if continue_sender {
						continue
					}

					sender_user = parseValue(sender_user, "<", ">")
					sender_user = strings.ReplaceAll(sender_user, "\u0000", "")
					sender_user = strings.ReplaceAll(sender_user, " ", "")

				}

				log.Println("SENDER USER: ", sender_user)

				if sender_user != "you" {

					user := sender_user
					sender_user = ""

					//log.Println("RECEIVE 2: ", string(data))

					if strings.Contains(string(data), "&") || continue_pid {
						pid_full, continue_pid = parse_message(data, `\&.*\$`, `\ \&.*`, `.*\$`)

						if continue_pid {
							continue
						}

						log.Println("PIIIIID: ", pid_full)

						if len(pid_full) > 0 {
							pid_parse := parseValue(string(pid_full), "&", "$")
							pid_parse = strings.ReplaceAll(pid_parse, "\u0000", "")
							pid_parse = strings.ReplaceAll(pid_parse, " ", "")
							if pid_parse != local_pid && remote_pid == "" {
								remote_pid = pid_parse
								BLEHandleFoundPeer(remote_pid)
								log.Println("PIIIIID: ", remote_pid)
							}
						}

					} else {

						message_full, continue_data = parse_message2(data, `\#.*\@`, `\ \#.*`, `.*\@`, `\#.*\%`)

						log.Println("continue_data: ", continue_data)
						log.Println("message_full: ", message_full)

						if continue_data || (message_full == "") {
							continue
						}

						parsedMessage, receiverUser := processMessage(message_full)

						b.logger.Info("received serial data")
						b.logger.Info(parsedMessage)

						if receiverUser != "" {
							fmt.Println("The message is send by: ", user, " and is send to: ", receiver_user)

						} else {
							fmt.Println("The message is send by: ", user, " and is send to me")

							fmt.Println("IS NOT REPEAT: ", searchMessage(parsedMessage) == -1)
							if searchMessage(parsedMessage) == -1 || (parsedMessage == "132f6d756c746973747265616d2f312e302e300a" && parsedMessage != message_parse_old) {

								message_parse_change := hexaToRune(parsedMessage)
								if message_parse_change != "" {
									BLEReceiveFromPeer(remote_pid, []byte(message_parse_change))
								}
							}
						}
						message_parse_old_list = append(message_parse_old_list, parsedMessage)
						message_parse_old = parsedMessage

					}

					fmt.Println("RESTARTING VARIABLES")

					data_parse = nil
					//dataParse = nil
					data_parse2 = nil
					sender_user = ""
					continue_sender = true
					message_full = ""
					continue_data = true

				} else {
					fmt.Println("The message is yours")

					sender_user = ""
					continue_sender = true
				}
			}
		}
	}()
}

func parseValue(value string, a string, b string) string {
	// Get substring between two strings.
	posFirst := strings.Index(value, a)
	if posFirst == -1 {
		return ""
	}
	posLast := strings.Index(value, b)
	if posLast == -1 {
		return ""
	}
	posFirstAdjusted := posFirst + len(a)
	if posFirstAdjusted >= posLast {
		return ""
	}
	return value[posFirstAdjusted:posLast]
}

func processMessage(message_full string) (string, string) {
	message_parse := strings.ReplaceAll(message_full, "#", "")
	message_parse = strings.ReplaceAll(message_parse, "@", "")
	message_parse = strings.ReplaceAll(message_parse, "%", "")
	message_parse = strings.ReplaceAll(message_parse, " ", "")
	receiver_user := parseValue(message_parse, "(", ")")
	receiver_user = strings.ReplaceAll(receiver_user, "\u0000", "")
	message_parse = strings.ReplaceAll(message_parse, "\u0000", "")
	message_parse = strings.Replace(message_parse, receiver_user, "", -1)
	message_parse = strings.Replace(message_parse, "()", "", -1)
	return message_parse, receiver_user
}

func searchMessage(s string) int {
	index := -1
	for i, v := range message_parse_old_list {
		if v == s {
			index = i
			break
		}
	}

	return index
}

/*func (b *Bridge) saveValue(mymessage string, key string) {
	//key = "test"
	ctx, cancel := context.WithCancel(context.Background())
	e := pubsubb.PutValue(ctx, "/v/"+key, []byte(mymessage))
	defer cancel()
	if e != nil {
		log.Fatalf("not value save: %v", e)
	}

	recover_value, e := pubsubb.GetValue(ctx, "/v/"+key)
	if e != nil {
		fmt.Println("not value: ", e)
	}
	fmt.Println("value saved: ", string(recover_value), " whith this key: ", key)

	log.Println("ARRIVAL TIME: ", time.Now())

	//protocolBridge.forwardMessage(string(recover_value), "0x0011")

}

func subscribe() {
	fmt.Println("SUBSCRIBE ")
	for {

		ch, err := pubsubb.SearchValue(context.Background(), "/v/test")

		if err != nil {
			continue
		}

		value := string(<-ch)

		fmt.Println("GOOOOT VALUE: ", value)

	}

}*/

func (b *Bridge) forwardMessage(mymessage string, key string) {
	b.logger.Info("writing data to serial interface:")
	//sendData = "chat msg #" + sendData + "@" + "<>" + "\n"
	sendData := "chat private " + key + " #" + mymessage + "@ \n"
	log.Println(sendData)
	//waiting_ack = true
	//time.Sleep(10 * time.Second)
	_, err := b.serial.Write([]byte(sendData))
	if err != nil && err != io.EOF {
		b.logger.Info("failed to write into serial interface", zap.Error(err))
		return
	}
	b.logger.Info("finished writing serial data")
}

// Close is used to shutdown the bridge serial interface
func (b *Bridge) Close() error {
	log.Println("PANIC2")
	return b.serial.Close()
}

func (b *Bridge) handleStream(s network.Stream) {
	b.logger.Info("Got a new stream!")

	// Create a buffer stream for non blocking read and write.
	//rw := bufio.NewReadWriter(bufio.NewReader(s), bufio.NewWriter(s))

	//go b.readData(rw)
	//go b.writeData(rw)

	// stream 's' will stay open until you close it (or the other side closes it).
}

func (b *Bridge) writeData(sendData []byte, wg *sync.WaitGroup, m *sync.Mutex) {
	//m.Lock()
	//stdReader := bufio.NewReader(os.Stdin)

	fmt.Println("SEND DATA BYTES: ", sendData)

	//data := runetoAscii(string(sendData))
	//sendDataChange, _ := changeCharacter(string(sendData))
	sendDataChange := runeToHexa(sendData)
	//done := make(chan error)

	//fmt.Println("SEND DATA LAST BYTES: ", sendData[len(sendData)-1])

	//if sendData[len(sendData)-1] == 10 {

	groupSize := 40
	for i := 0; i < len(sendDataChange); i += groupSize {

		end := i + groupSize
		if end > len(sendDataChange) {
			end = len(sendDataChange)
		}

		fmt.Println("SEND DATA STRING " + strconv.Itoa(i/groupSize) + " of " + strconv.Itoa(len(sendDataChange)/groupSize))

		if end == len(sendDataChange) {
			s1 := []byte("chat msg #")
			s2 := []byte("@ \n")

			sendData := append([]byte(sendDataChange[i:end]), s2...)

			sendData = append(s1, sendData...)

			//fmt.Println("SEND DATA STRING: ", string(sendData))

			b.logger.Info("writing data to serial interface:")

			_, err := b.serial.Write(sendData)
			if err != nil && err != io.EOF {
				b.logger.Error("failed to write into serial interface", zap.Error(err))
				return
			}

		} else {
			fmt.Println(sendDataChange[i:end])

			s1 := []byte("chat msg #")
			s2 := []byte("% \n")

			sendData := append([]byte(sendDataChange[i:end]), s2...)

			sendData = append(s1, sendData...)

			//fmt.Println("SEND DATA STRING: ", string(sendData))

			b.logger.Info("writing data to serial interface:")

			_, err := b.serial.Write(sendData)
			if err != nil && err != io.EOF {
				b.logger.Error("failed to write into serial interface", zap.Error(err))
				return
			}
			//done := make(chan error)
		}

		time.Sleep(5 * time.Second)

	}

}

/*func (b *Bridge) writeData2(rw *bufio.ReadWriter) {
	stdReader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		sendData, err := stdReader.ReadString('\n')
		sendData = strings.TrimSuffix(sendData, "\n")
		if err != nil {
			log.Println(err)
			return
		}

		fmt.Println("you sent this: ", sender_user)

		if sendData == "load2" {
			ctx := context.Background()

			recover_value, e := pubsubb.GetValue(ctx, "/v/0x0004")
			if e != nil {
				fmt.Println("not value: ", e)
				return
			}
			fmt.Println(string(recover_value))
			return
		}

		b.logger.Info("writing data to serial interface:")
		sendData = "chat msg #" + sendData + "@ \n"
		//log.Println(sendData)
		_, err = b.serial.Write([]byte(sendData))
		if err != nil && err != io.EOF {
			b.logger.Error("failed to write into serial interface", zap.Error(err))
			return
		}
	}
}*/

func runeToHexa(b []byte) string {
	hexStr := hex.EncodeToString(b)
	fmt.Println("Hexadecimal representation:", hexStr)
	return hexStr
}

func hexaToRune(hexStr string) string {
	str, _ := hex.DecodeString(hexStr)

	fmt.Println("String:", string(str))
	return string(str)
}

func (b *Bridge) writePID(pid string) {
	//stdReader := bufio.NewReader(os.Stdin)

	b.logger.Info("writing PID to serial interface:")
	pid = "chat msg &" + pid + "$\n"
	log.Println(pid)
	_, err := b.serial.Write([]byte(pid))
	if err != nil && err != io.EOF {
		b.logger.Error("failed to write into serial interface", zap.Error(err))
		return
	}
}

/*func (b *Bridge) readData(rw *bufio.ReadWriter) {
	for {
		str, _ := rw.ReadString('\n')

		if str == "" {
			return
		}
		if str != "\n" {
			fmt.Printf("\x1b[32m%s\x1b[0m> ", str)

			if kademliaDHT != nil {

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				e := kademliaDHT.PutValue(ctx, "/v/hello", []byte("valid"))
				if e != nil {
					fmt.Println("not value save: ", e)
				}

				recover_value, e := kademliaDHT.GetValue(ctx, "/v/hello")
				if e != nil {
					fmt.Println("not value: ", e)
				}
				fmt.Println(string(recover_value))
			}
		}

	}
}*/

func parse_message(message []byte, ex1 string, ex2 string, ex3 string) (string, bool) {

	r1, _ := regexp.Compile(ex1)

	matched1 := r1.MatchString(string(message))

	if !matched1 {

		r2, _ := regexp.Compile(ex2)

		r3, _ := regexp.Compile(ex3)

		matched2 := r2.MatchString(string(message))
		matched3 := r3.MatchString(string(message))

		if !matched3 {
			if matched2 {
				//fmt.Println("part1", string(message))
				data_parse = r2.Find(message)
			} else if data_parse != nil {
				//fmt.Println("part2", string(message))
				data_parse = append(data_parse, message...)
			}
			return string(data_parse), true
		} else if len(data_parse) == 0 {
			return string(data_parse), true
		}

		//fmt.Println(r2.FindString(string(data)))

		data_parse = append(data_parse, r3.Find(message)...)

	} else {
		data_parse = r1.Find(message)
	}

	return string(data_parse), false

}

func parse_message2(message []byte, ex1, ex2, ex3, ex4 string) (string, bool) {
	var dataParse []byte

	r1 := regexp.MustCompile(ex1)
	if r1.MatchString(string(message)) {
		if string(dataParseFin) != string(r1.Find(message)) {
			dataParseFin = r1.Find(message)
			data_parse2 = append(data_parse2, r1.Find(message)...)
			return string(data_parse2), false
		}
		dataParseFin = nil
		return "", false
	}

	r2 := regexp.MustCompile(ex2)
	r3 := regexp.MustCompile(ex3)
	r4 := regexp.MustCompile(ex4)

	if r3.MatchString(string(message)) {
		if string(dataParseFin) != string(r3.Find(message)) {
			dataParseFin = r3.Find(message)
			fmt.Println("part4", string(r3.Find(message)))
			data_parse2 = append(data_parse2, r3.Find(message)...)
			return string(data_parse2), false
		}
		dataParseFin = nil
		return "", false
	}

	if r4.MatchString(string(message)) {
		//fmt.Println("part2", string(r4.Find(message)))
		//data_parse2 = append(data_parse2, r4.Find(message)...)
		dataParse = r4.Find(message)
	} else if r2.MatchString(string(message)) {
		fmt.Println("part1", string(r2.Find(message)))
		//data_parse2 = append(data_parse2, r2.Find(message)...)
		dataParse = r2.Find(message)
	} else {
		//fmt.Println("part3", string(message))
		//data_parse2 = append(data_parse2, message...)
		//dataParse = message
	}

	fmt.Println("DATA: ", string(dataParse), "DATA_OLD: ", message_full_old)

	if string(dataParse) == message_full_old {
		return string(data_parse2), true
	}

	if string(dataParse) != "" {
		message_full_old = string(dataParse)
	}

	data_parse2 = append(data_parse2, dataParse...)

	fmt.Println("parse_message RETURN: ", string(data_parse2))

	return string(data_parse2), true
}
