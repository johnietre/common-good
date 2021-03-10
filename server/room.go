package main

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	webs "golang.org/x/net/websocket"
)

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
	defer func() { // Clean up after game end
		logger.Printf("Room %s ended", room.id)
		msg := Message{
			Action:   "ended",
			Contents: "game ended",
			Sender:   "server",
		}
		room.broadcast(msg, func(m *Member) {
			m.ws.Close()
		})
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

		/* Possibly unneeded; check how deadline works if msg recv before deadline
		// Set the deadlines for the conns as the end of the chat phase
		for _, member := range room.Members {
			member.ws.SetReadDeadline(time.Unix(room.NextPhase, 0))
		}
		*/

		// Listen for new messages until the end of the phase
		for phaseEnd.After(time.Now()) {
			msg := <-room.hub
			for _, member := range msg.Recipients {
				if err := webs.JSON.Send(room.Members[member].ws, *msg); err != nil {
					/* Handle error */
					log.Println(err)
				}
			}
		}
		// Create set order for members
		members := make([]*Member, 0, len(room.Members))
		for _, member := range room.Members {
			members = append(members, member)
		}
		// Start the individual/deposit phase
		var tax, funds int32
		for _, member := range members {
			name := member.Name
			room.depositTurn = member
			msg.Action = "started"
			msg.Sender = "server"
			msg.Contents = name + "'s turn"
			room.broadcast(msg, nil)
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
			msg.Action = "ended"
			msg.Sender = "server"
			msg.Contents = name + "'s turn is over"
			room.broadcast(msg)
		}
		room.depositTurn = nil
		// Distribute the tax to the members
		totalTax := tax * 2
		tax = totalTax / room.Joined
		msg.Action = "deposited"
		msg.Sender = "server"
		msg.Contents = fmt.Sprintf("Total tax contributed: %d, %d coins go to each player", totalTax, tax)
		room.broadcast(msg, func(m *Member) {
			m.Funds += tax
		})
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

func (room *Room) broadcast(msg Message, f func(*Member)) {
	for name, member := range room.Members {
		if name != msg.Sender {
			/* TODO: Handle error */
			webs.JSON.Send(member.ws, &msg)
			if f != nil {
				f(member)
			}
		}
	}
}
