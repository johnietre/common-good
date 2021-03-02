/* TODO
 * Handle SetDeadline errors
 * Give members a set order since maps are unordered
 */

/* Notes
 * Using 3 minute convo time for rooms
 * Using room capacity of 3
 * Running for 3 rounds
 * Private time of 1 minute
 */

/* Ideas
* Possibly have the socketHAndler keep running handling the websocket
	and give the room an property telling at what time the socket should
	stop waiting for reads, therefore the SetReadDeadline will always end
	at the correct time (is updated after each read)
	 * Doing this decreases the amount of goroutines running
* Possibly have it so that during chat time, the members are looped through
	with a read deadline of a millisecond
*/

package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	webs "golang.org/x/net/websocket"
)

const (
	ip   string = "192.168.1.125"
	port string = "8000"
)

var (
	temp   *template.Template
	logger = log.New(os.Stdout, "", log.LstdFlags)
)

func main() {
	temp = template.Must(template.ParseFiles("../templates/index.html"))

	static := http.FileServer(http.Dir("../static"))
	http.Handle("/static/", http.StripPrefix("/static", static))
	http.HandleFunc("/", pageHandler)
	http.Handle("/socket/", webs.Handler(socketHandler))
	panic(http.ListenAndServe(ip+":"+port, nil))
}

func pageHandler(w http.ResponseWriter, r *http.Request) {
	temp.Execute(w, nil)
}

// Handle the real-time connections
func socketHandler(ws *webs.Conn) {
	// defer ws.Close()
	var room *Room
	var msg *Message
	var err error
	// Get the room (room search page)
	for room == nil {
		if err = webs.JSON.Receive(ws, msg); err != nil {
			logger.Println(err)
			ws.Close()
			return
		}
		if msg.Action == "create" {
			room = createRoom()
			if room == nil {
				msg.Action = "error"
				msg.Contents = "error creating room"
				webs.JSON.Send(ws, msg)
				continue
			}
			// msg.Action = "created"
			// msg.Contents = room.id
			// if err := webs.JSON.Send(ws, msg); err != nil {
			//   logger.Println(err)
			//   ws.Close()
			//   return
			// }
			break
		}
		room, err = getRoom(msg.Contents)
		if err != nil {
			msg.Action = "error"
			msg.Contents = err.Error()
			if err := webs.JSON.Send(ws, msg); err != nil {
				logger.Println(err)
				ws.Close()
				return
			}
		} else if room == nil {
			msg.Action = "error"
			msg.Contents = "room doesn't exist"
			if err := webs.JSON.Send(ws, msg); err != nil {
				logger.Println(err)
				ws.Close()
				return
			}
		}
	}
	msg.Action = "joined"
	msg.Contents = room.id
	if err = webs.JSON.Send(ws, msg); err != nil {
		logger.Println(err)
		return
	}

	// Add user to members
	if err = webs.JSON.Receive(ws, msg); err != nil {
		logger.Println(err)
		return
	}
	member, err := room.addMember(msg.Contents, ws)
	for err != nil {
		msg.Action = "error"
		msg.Contents = err.Error()
		webs.JSON.Send(ws, msg)
		if err = webs.JSON.Receive(ws, msg); err != nil {
			logger.Println(err)
			return
		}
		member, err = room.addMember(msg.Contents, ws)
	}
	msg.Action = "added"
	webs.JSON.Send(ws, msg)
	// Listen for and filter messages from the member
	for room.Round <= room.Rounds {
		if err := webs.JSON.Receive(ws, msg); err != nil {
			if strings.Contains(err.Error(), "timeout") {
				continue
			} else if strings.Contains(err.Error(), "closed") {
				return
			}
			log.Println(err)
		}
		if (msg.Action == "chat" && room.depositTurn == nil) || (msg.Action == "deposit" && room.depositTurn == member) {
			// TODO: Make it so that if the msg is overwritten outside of the hub/function,
			// the msg contents won't be changed
			room.hub <- msg
		}
	}
}

// Message holds JSON messages/actions as well as chat message info
type Message struct {
	// Actions:
	// "error"
	// "create"
	// "joined"
	// "added"
	// "started"
	// "chat"
	// "turn start"
	// "deposit"
	// "turn end"
	// "ended"
	Action     string   `json:"action"`
	Contents   string   `json:"contents"`
	Sender     string   `json:"sender"`
	Recipients []string `json:"recipients"`
	Timestamp  int64    `json:"timestamp"`
}

// Room is what holds members as well as messages
type Room struct {
	Members     map[string]*Member
	Messages    []Message
	Connected   int32 // Number of people who get the room
	Joined      int32 // Number of members actually joined
	Capacity    int32
	Round       int32
	Rounds      int32 // Total number of rounds in game
	MaxCoins    int32
	NextPhase   int64 // unix time
	id          string
	depositTurn *Member
	chatTime    time.Duration
	privateTime time.Duration
	hub         chan *Message
	sync.RWMutex
}

var ( // possibly use sync.Map
	rooms     map[string]*Room = make(map[string]*Room) // [UUID]Room
	roomsLock sync.RWMutex
)

func createRoom() *Room {
	roomsLock.Lock()
	defer roomsLock.Unlock()
	var uuid string
	for {
		buuid, err := exec.Command("uuidgen").Output()
		if err != nil {
			logger.Println(err)
			return nil
		}
		uuid = string(buuid[:8])
		if _, ok := rooms[uuid]; !ok {
			break
		}
	}
	room := &Room{
		Members:     make(map[string]*Member),
		Capacity:    3,
		Rounds:      3,
		id:          uuid,
		chatTime:    3,
		privateTime: 1,
		hub:         make(chan *Message, 10),
	}
	rooms[uuid] = room
	go room.run()
	return room
}

// Possibly check for full room here
func getRoom(roomID string) (*Room, error) {
	roomsLock.RLock()
	defer roomsLock.RUnlock()
	room := rooms[roomID]
	if room == nil {
		return nil, errors.New("room doesn't exist")
	} else if !room.connect() {
		return nil, errors.New("room full")
	}
	return room, nil
}

func (room *Room) run() error {
	// Wait for every member to join
wait:
	for atomic.LoadInt32(&room.Joined) < room.Capacity {
	}
	logger.Println("Starting room in 20s")
	time.Sleep(time.Second * 20)
	allGood := true
	for _, member := range room.Members {
		msg := &Message{Action: "started"}
		if err := webs.JSON.Send(member.ws, msg); err != nil {
			logger.Println(err)
			member.ws.Close()
			atomic.AddInt32(&room.Connected, -1)
			atomic.AddInt32(&room.Joined, -1)
			room.Lock()
			delete(room.Members, member.Name)
			room.Unlock()
			allGood = false
		}
	}
	if !allGood {
		goto wait
	}
	// Start the room
	logger.Println("Starting room:", room.id)
	time.Sleep(time.Minute)
	defer func() {
		logger.Printf("Room %s ended", room.id)
		for _, member := range room.Members {
			msg := &Message{Action: "ended"}
			webs.JSON.Send(member.ws, msg)
			member.ws.Close()
		}
		roomsLock.Lock()
		delete(rooms, room.id)
		roomsLock.Unlock()
	}()

	// Game code
	var msg *Message
	for room.Round = 1; room.Round <= room.Rounds; room.Round++ {
		// Set the time for the end of the chat phase
		phaseEnd := time.Now().Add(time.Minute * room.chatTime)
		room.NextPhase = phaseEnd.Unix()
		// Set the deadlines for the conns as the end of the chat phase
		for _, member := range room.Members {
			member.ws.SetReadDeadline(time.Unix(room.NextPhase, 0))
		}
		// Listen for new messages until the end of the phase
		for phaseEnd.After(time.Now()) {
			msg := <-room.hub
			for _, member := range msg.Recipients {
				if err := webs.JSON.Send(room.Members[member].ws, msg); err != nil {
					/* Handle error */
					log.Println(err)
				}
			}
		}
		// Start the individual/deposit phase
		var tax, funds int32
		/* TODO^ */
		for name, member := range room.Members {
			room.depositTurn = member
			msg.Action = "turn start"
			msg.Contents = name + "'s turn"
			for _, m := range room.Members {
				webs.JSON.Send(m.ws, &msg)
			}
			// Wait for the member to allocate funds
			/* Handle error from SetReadDeadline */
			member.ws.SetReadDeadline(time.Now().Add(time.Minute * room.privateTime))
			if err := webs.JSON.Receive(member.ws, msg); err != nil {
				logger.Println(err)
				/* Handle rest of error */
			}
			// Get funds and add them to the personal funds and tax
			funds64, err := strconv.ParseInt(msg.Contents, 10, 32)
			if err != nil {
				logger.Println(err)
				/* Handle rest of error */
				funds = 0
			}
			funds = int32(funds64)
			member.Funds += funds
			tax += room.MaxCoins - funds
			msg.Action = "turn end"
			msg.Contents = name + "'s turn is over"
			for _, m := range room.Members {
				webs.JSON.Send(m.ws, &msg)
			}
		}
		room.depositTurn = nil
		// Distribute the tax to the members
		tax = tax * 2 / room.Joined
		msg.Action = "deposit"
		msg.Sender = "server"
		msg.Contents = fmt.Sprintf("Total tax contributed: %d, %d coins go to each player", tax*room.Joined, tax)
		for _, member := range room.Members {
			if err := webs.JSON.Send(member.ws, msg); err != nil {
				/* Handle error */
				log.Println(err)
			}
			member.Funds += tax
		}
	}
	return nil
}

func (room *Room) connect() bool {
	if atomic.AddInt32(&room.Connected, 1) > room.Capacity {
		atomic.AddInt32(&room.Connected, -1)
		return false
	}
	return true
}

func (room *Room) disconnect(ws *webs.Conn) {
	room.Lock()
	defer room.Unlock()
	for name, member := range room.Members {
		if member.ws == ws {
			delete(room.Members, name)
			room.Connected--
			return
		}
	}
}

func (room *Room) addMember(name string, ws *webs.Conn) (*Member, error) {
	room.Lock()
	defer room.Unlock()
	if _, ok := room.Members[name]; ok {
		return nil, errors.New("member already exists")
	}
	room.Members[name] = &Member{Name: name, ws: ws}
	logger.Printf("Added %s to room %s\n", name, room.id)
	return room.Members[name], nil
}

func (room *Room) removeMember(name string, ws *webs.Conn) error {
	room.Lock()
	defer room.Unlock()
	if ws == nil {
		for name, member := range room.Members {
			if member.ws == ws {
				delete(room.Members, name)
				return nil
			}
		}
	}
	delete(room.Members, name)
	return nil
}

// Member holds room member info
type Member struct {
	Name   string
	RoomID string `json:"roomid"`
	Funds  int32  `json:"funds"`
	ws     *webs.Conn
	// ip string
}
