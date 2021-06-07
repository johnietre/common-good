/* TODO
 * Handle SetDeadline errors
 * Look at reworking room joining process in socketHandler
 */

/* Notes
 * Using 3 minute convo time for rooms
 * Using room capacity of 3
 * Running for 3 rounds
 * Private time of 1 minute
 */

/* Ideas
 * Possibly have the socketHandler keep running handling the websocket
	 and give the room an property telling at what time the socket should
	 stop waiting for reads, therefore the SetReadDeadline will always end
	 at the correct time (is updated after each read)
 	 * Doing this decreases the amount of goroutines running
 * Possibly have it so that during chat time, the members are looped through
	 with a read deadline of a millisecond
 * Possibly broadcast all chat messages (client JS will show a message ws sent)
	 between the recipients but won't show the message
*/

package main

import (
	"html/template"
	"log"
	"net/http"
	"os"

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

	server := &http.Server{
		Addr:     ip + ":" + port,
		Handler:  routes(),
		ErrorLog: logger,
	}
	panic(server.ListenAndServe())
}

func routes() *http.ServeMux {
	r := http.NewServeMux()
	static := http.FileServer(http.Dir("../static"))
	r.Handle("/static/", http.StripPrefix("/static", static))
	r.HandleFunc("/", pageHandler)
	r.Handle("/socket/", webs.Handler(socketHandler))
	return r
}

func pageHandler(w http.ResponseWriter, r *http.Request) {
	if err := temp.Execute(w, nil); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		logger.Println(err)
	}
}

// Handle the real-time connections
func socketHandler(ws *webs.Conn) {
	var closePtr *bool
	*closePtr = true
	defer func() {
		if *closePtr {
			ws.Close()
		}
	}()
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
				webs.JSON.Send(ws, newErrMsg("error creating room"))
			} else {
				msg = &Message{Action: "create", Contents: room.id}
				if err := webs.JSON.Send(ws, msg); err != nil {
					logger.Println(err)
					return
				}
			}
		} else if msg.Action == "join" {
			room, err = getRoom(msg.Contents)
			if err != nil {
				if err := webs.JSON.Send(ws, newErrMsg(err.Error())); err != nil {
					logger.Println(err)
					return
				}
			} else {
				msg = &Message{Action: "join", Contents: "room joined"}
				if err := webs.JSON.Send(ws, msg); err != nil {
					logger.Println(err)
					return
				}
			}
		} else {
			if err := webs.JSON.Send(ws, newErrMsg("invalid action")); err != nil {
				logger.Println(err)
				return
			}
		}
	}

	var member *Member
	for member == nil {
		if err = webs.JSON.Receive(ws, msg); err != nil {
			logger.Println(err)
			return
		}
		if msg.Action != "join" {
			if err := webs.JSON.Send(ws, newErrMsg("invalid action")); err != nil {
				logger.Println(err)
				return
			}
		} else if member, err = room.addMember(msg.Contents, ws); err != nil {
			if err := webs.JSON.Send(ws, newErrMsg(err.Error())); err != nil {
				logger.Println(err)
				return
			}
		}
	}
	msg = &Message{Action: "join", Contents: "joined"}
	if err = webs.JSON.Send(ws, msg); err != nil {
		logger.Println(err)
		return
	}
	*closePtr = false

	/*
		// Listen for and filter messages from the member
		for room.getRound() <= room.Rounds {
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
	*/
}

// Message holds JSON messages/actions as well as chat message info
type Message struct {
	/* Actions
	 * error: an error has occured (server or user)
	 * create: deals with room creation
	 * joined: the user has joined the room
	 * added: the user's name has been added to the room's roster
	 * start: game, round, or turn has started
	 * chat: a chat message
	 * deposited: funds have been deposited
	 * ended: game, round, or turn has ended
	 */
	Action     string   `json:"action"`
	Contents   string   `json:Contents:`
	Sender     string   `json:"sender"`
	Recipients []string `json:"recipients"`
	Timestamp  int64    `json:"timestamp"`
}

func newErrMsg(contents string) *Message {
	return &Message{
		Action:   "error",
		Contents: contents,
		Sender:   "server",
	}
}

// Member holds room member info
type Member struct {
	Name   string `json:"name"`
	RoomID string `json:"roomid"` /* TODO: Make int32 and use math.rand */
	Funds  int32  `json:"funds"`
	ws     *webs.Conn
	// ip string
}
