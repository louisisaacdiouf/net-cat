package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

type Server struct {
	listenAddr string
	ln         net.Listener
	quitch     chan struct{}
	msgch      chan []byte
	clients    map[net.Conn]string
}

var NetCatServer Server
var ExistingUsers = make(map[string]bool)
var maxUsers = 10

var MsgLog []Msg

func NewServer(listenAddr string) *Server {
	return &Server{
		listenAddr: listenAddr,
		quitch:     make(chan struct{}),
		msgch:      make(chan []byte, 10),
		clients:    make(map[net.Conn]string),
	}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()
	s.ln = ln
	fmt.Println("Listening on the port :", s.listenAddr[len("localhost:"):])

	go s.acceptLoop()

	<-s.quitch
	close(s.msgch)

	return nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}

		go s.ShowLogin(conn)

		go s.readLoop(conn)

		go s.printLoop(conn)

	}
}

func (s *Server) readLoop(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 4096)
	msgCount := 0
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				name := s.clients[conn]

				newMsg := Msg{"notif", name, name + " has left our chat...", time.Now().Format("2006-01-02 15:04:05")}
				req, err := json.Marshal(newMsg)
				LogError(err)
				if len(newMsg.Author) > 0 {
					s.msgch <- req
					s.closeConnection(conn, newMsg.Author)
				}
			} else {
				fmt.Println("read error:", err)
			}
			break
		}

		msg := buf[:n]
		if msgCount == 0 { // First Message = Username
			msgTxt := string(msg)
			s.AddClient(conn, msgTxt)
		} else {
			s.msgch <- msg
		}
		msgCount++
	}
}

func (s *Server) closeConnection(conn net.Conn, client string) {
	delete(s.clients, conn)
	ExistingUsers[client] = false
	conn.Close()
}

func (s *Server) printLoop(conn net.Conn) {
	for msg := range s.msgch {
		newMSG := Msg{}
		err := json.Unmarshal(msg, &newMSG)
		LogError(err)

		Colorize(&newMSG)
		MsgLog = append(MsgLog, newMSG)

		if len(newMSG.Text) > 0 {
			if newMSG.Type != "msg" {
				fmt.Println(newMSG.Text)
			}
			s.BroadcastMsg(EncodeMsg(newMSG), newMSG.Author)
		}
	}
}

func (s *Server) ShowLogin(conn net.Conn) error {
	// Display welcome text
	welcomeText, err := os.ReadFile("welcome-text.txt")
	if err != nil {
		return errors.New("don't delete or rename \033[31mwelcome-text.txt\033[00m file")
	}
	_, err = conn.Write(welcomeText)
	if err != nil {
		log.Fatal(err)
	}
	return nil
}

func (s *Server) AddClient(conn net.Conn, name string) {
	if !ExistingUsers[name] && len(s.clients) < maxUsers {
		s.clients[conn] = name     // Save client
		ExistingUsers[name] = true // Mark client as existing

		// Send message to client
		s.MsgToClient("notif", name+" has joined our chat...", time.Now().Format("2006-01-02 15:04:05"), conn)

		// Send logs
		if len(MsgLog) > 0 {
			s.MsgToClient("logs", "Read the log file", time.Now().Format("2006-01-02 15:04:05"), conn)
		}
	} else if ExistingUsers[name] {
		s.MsgToClient("error", "That username already exists.", time.Now().Format("2006-01-02 15:04:05"), conn)
	} else if len(s.clients) == 8 {
		s.MsgToClient("error", "Max number of users reached.", time.Now().Format("2006-01-02 15:04:05"), conn)
	}
}

func (s *Server) BroadcastMsg(msg []byte, excluded string) {
	for conn, usr := range s.clients {
		if usr != excluded {
			conn.Write([]byte(msg))
		}
	}
}

func MsgLogsToText(logs []Msg) string {
	var txt string
	for _, msg := range logs {
		if msg.Type == "msg" {
			txt += Blue + UserMsgDate(msg.Author, msg.Date) + ColorAnsiEnd
		}
		txt += msg.Text
		if msg.Type == "error" || msg.Type == "notif" {
			txt += "\n"
		}
	}
	return txt
}

func (s *Server) MsgToClient(typeMsg, txt, t string, conn net.Conn) {
	name := s.clients[conn]
	newMsg := Msg{typeMsg, name, txt, time.Now().Format("2006-01-02 15:04:05")}

	Colorize(&newMsg)

	req := EncodeMsg(newMsg)

	if typeMsg == "error" {
		fmt.Println(newMsg.Text)
		conn.Write(req)
	} else if typeMsg == "logs" {
		logs, err := json.Marshal(MsgLog)
		LogError(err)
		err = os.WriteFile("msglogs.json", logs, 0755)
		LogError(err)
		conn.Write(req)
	} else {
		s.msgch <- req
	}
}

var Orange = ColorAnsiStart(255, 94, 0)
var Red = ColorAnsiStart(255, 0, 0)
var Blue = ColorAnsiStart(0, 60, 255)

func Colorize(msg *Msg) {
	switch msg.Type {
	case "notif":
		msg.Text = Orange + msg.Text + ColorAnsiEnd
	case "error":
		msg.Text = Red + msg.Text + ColorAnsiEnd
	case "msg":
		msg.Text = Blue + msg.Text + ColorAnsiEnd + "\n"
	}
}

// Function that creates the escape color string for the given RGB color
func ColorAnsiStart(R, G, B int) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", R, G, B)
}

// Color string to reset string color
var ColorAnsiEnd = "\033[0m"
