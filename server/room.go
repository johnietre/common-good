package main

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
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
	TotalRounds int32
	MaxCoins    int32
	NextPhase   int64 // unix time
	id          string
	depositTurn *Member
	chatTime    time.Duration
	privateTime time.Duration
	hub         chan *Message
	sync.RWMutex
}

var (
	rooms sync.Map // [UUID]*Room
)

func createRoom() *Room {
	room := &Room{
		Members:     make(map[string]*Member),
		Capacity:    3,
		TotalRounds: 3,
		chatTime:    3,
		privateTime: 1,
		hub:         make(chan *Message, 10),
	}
	for {
		bid, err := uuid.NewRandom()
		if err != nil {
			logger.Println(err)
			return nil
		}
		room.id = bid.String()
		if _, loaded := rooms.LoadOrStore(room.id, room); !loaded {
			break
		}
	}
	go room.run()
	return room
}

// Possibly check for full room here
func getRoom(roomID string) (*Room, error) {
	iRoom, ok := rooms.Load(roomID)
	if !ok {
		return nil, errors.New("room doesn't exist")
	}
	room := iRoom.(*Room)
	if !room.connect() {
		return nil, errors.New("room full")
	}
	return room, nil
}

// Start the game room
func (room *Room) run() error {
	// Wait for every member to join
wait:
	for room.getJoined() < room.Capacity {
	}
	logger.Println("Starting room in 20s")
	time.Sleep(time.Second * 20)
	allGood := true
	for _, member := range room.Members {
		msg := &Message{Action: "start"}
		if err := webs.JSON.Send(member.ws, msg); err != nil {
			logger.Println(err)
			member.ws.Close()
			room.decrJoined()
			room.decrConnected()
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
		rooms.Delete(room.id)
	}()

	// Game code
	var msg *Message
	for room.Round = 1; room.Round <= room.TotalRounds; room.Round++ {
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
			msg.Action = "start"
			msg.Sender = "server"
			msg.Contents = name + "'s turn"
			room.broadcast(*msg, nil)
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
			room.broadcast(*msg, nil)
		}
		room.depositTurn = nil
		// Distribute the tax to the members
		totalTax := tax * 2
		tax = totalTax / room.Joined
		msg.Action = "deposited"
		msg.Sender = "server"
		msg.Contents = fmt.Sprintf("Total tax contributed: %d, %d coins go to each player", totalTax, tax)
		room.broadcast(*msg, func(m *Member) {
			m.Funds += tax
		})
	}
	return nil
}

func (room *Room) connect() bool {
	if room.incrConnected() > room.Capacity {
		room.decrConnected()
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

func (r *Room) getRound() int32 {
	return atomic.LoadInt32(&r.Round)
}

func (r *Room) incrRound() int32 {
	return atomic.AddInt32(&r.Round, 1)
}

func (r *Room) getJoined() int32 {
	return atomic.LoadInt32(&r.Joined)
}

func (r *Room) incrJoined() int32 {
	return atomic.AddInt32(&r.Joined, 1)
}

func (r *Room) decrJoined() int32 {
	return atomic.AddInt32(&r.Joined, -1)
}

func (r *Room) getConnected() int32 {
	return atomic.LoadInt32(&r.Connected)
}

func (r *Room) incrConnected() int32 {
	return atomic.AddInt32(&r.Connected, 1)
}

func (r *Room) decrConnected() int32 {
	return atomic.AddInt32(&r.Connected, -1)
}
