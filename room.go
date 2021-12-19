package main

import (
  "errors"
  "sync"

  "github.com/google/uuid"
  webs "golang.org/x/net/websocket"
)

const (
  roomIdLength = 8
  chatChanLen = 10
)

var (
  ErrMemberExists = errors.New("member already exists")
  ErrRoomFull = errors.New("room full")
)

type Member struct {
  Name string `json:"name"`
  ws *webs.Conn
}

type Room struct {
  // 8 character UUID
  RoomId string `json:"roomId"`

  MaxMembers int `json:"maxMembers"`
  // members []*Member
  // memberMap map[string]int // map[member.Name]i where members[i] = member
  // map[Member.Name]*Member
  Members map[string]*Member `json:"members"`
  // map[Member.Name]coins
  coins map[string]int

  Turn string `json:"turn"` // member.Name
  Round int `json:"round"`
  TotalRounds int `json:"totalRounds"`

  chatChan chan Message

  mtx sync.RWMutex
}

func NewRoom() (*Room, error) {
  id, err := uuid.NewUUID()
  if err != nil {
    return nil, err
  }
  return &Room{
    RoomId: id.String()[:roomIdLength],
    MaxMembers: 5,
    Members: make(map[string]*Member, 5),
    TotalRounds: 5,
    chatChan: make(chan Message, chatChanLen),
  }, nil
}

func NewRoomMust() *Room {
  return  &Room{
    RoomId: uuid.New().String()[:roomIdLength],
    MaxMembers: 5,
    Members: make(map[string]*Member, 5),
    TotalRounds: 5,
    chatChan: make(chan Message, chatChanLen),
  }
}

func RoomWith(maxMembers, totalRounds int) *Room {
  return &Room{
    RoomId: uuid.New().String()[:roomIdLength],
    MaxMembers: maxMembers,
    Members: make(map[string]*Member, maxMembers),
    TotalRounds: totalRounds,
    chatChan: make(chan Message, chatChanLen),
  }
}

func (r *Room) AddMember(mem *Member) error {
  r.mtx.Lock()
  defer r.mtx.Unlock()
  if len(r.Members) == r.MaxMembers {
    return ErrRoomFull
  }
  if _, ok := r.Members[mem.Name]; ok {
    return ErrMemberExists
  }
  r.Members[mem.Name] = mem
  return nil
}

func (r *Room) RemoveMember(mem *Member) {
  r.mtx.Lock()
  defer r.mtx.Unlock()
  delete(r.Members, mem.Name)
}

func (r *Room) Run() {
  //
}

func (r *Room) Broadcast(msg Message) {
  r.chatChan <- msg
}

func (r *Room) runChat() {
  msgNum := 0
  for msg := range r.chatChan {
    msgNum++
    msg.Id = msgNum
    for _, name := range msg.Recipients {
      mem := r.Members[name]
      if err := webs.JSON.Send(mem.ws, msg); err != nil {
        // TODO: Handle error
        logger.Println(err)
      }
    }
  }
}

func (r *Room) gameInProgress() bool {
  return r.Round != 0 && r.Round <= r.TotalRounds
}

type MessageAction string

const (
  // IDEA: Add actions for game force end, game over (won/tie)
  // and reduce scope of "leave"

  // Signifies a user joining a room
  // Expects room ID as contents
  // Sends the full JSON room information as contents
  ActionJoin MessageAction = "join"
  // Signifies a user creating a room
  // Expects JSON room as contents
  // Sends the full JSON room information as contents
  ActionCreate MessageAction = "create"
  // Signifies a name submitted by a user
  // Expects name as contents
  // Sends with an empty error on success
  ActionName MessageAction = "name"
  // Signifies a chat message
  // Expects a sender, recipients, and contents
  // Sends (broadcasts) the message to the recipients, including the sender
  ActionChat MessageAction = "chat"
  // Signifies the start of a game
  // Sends an empty message
  ActionGameStart MessageAction = "gameStart"
  // Signifies the start of a round
  // Sends the round number as the contents?
  ActionRoundStart MessageAction = "roundStart"
  // Signifies a turn change
  // Sends the name of the player whose turn it is
  ActionStartTurn MessageAction = "turnStart"
  // Signifies a player depositing coins, and thus the end of a turn
  // Expects the number of coins in contents
  // Sends with an empty error on success
  ActionDeposit MessageAction = "deposit"
  // Signifies a vote on which player to vote out
  // Expects the name of the player to vote out, or nothing
  // Sends with an empty error on success; if a player is voted out, their name
  // is sent, otherwise, an empty contents is sent
  ActionVote MessageAction = "vote"
  // Signifies the end of a round
  // Sends the amount of coins collected as tax and how it's divied up
  ActionRoundEnd MessageAction = "roundEnd"
  // Signifies a user leaving the game
  // Expects a message
  // Sends an empty message
  ActionLeave MessageAction = "leave"
  // Signifies the end of the game
  // Sends the name(s) of the winner(s) as recipients (still sent to all)
  // If no one won, recipients is empty
  ActionGameEnd MessageAction = "gameEnd"
  // Signifies an error from the server
  // Sends error information in the error and contents fields
  ActionError MessageAction = "error"
)

type Message struct {
  Id int `json:"id"`
  Action MessageAction `json:"action"`
  Sender string `json:"sender"`
  Recipients []string `json:"recipients"`
  Contents string `json:"contents"`
  Error string `json:"error"`
}
