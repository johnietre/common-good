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
* Possibly have the socketHandler keep running handling the websocket
	and give the room an property telling at what time the socket should
	stop waiting for reads, therefore the SetReadDeadline will always end
	at the correct time (is updated after each read)
	 * Doing this decreases the amount of goroutines running
* Possibly have it so that during chat time, the members are looped through
	with a read deadline of a millisecond
*/

package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

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

// Member holds room member info
type Member struct {
	Name   string `json:"name"`
	RoomID string `json:"roomid"` /* TODO: Make int32 and use math.rand */
	Funds  int32  `json:"funds"`
	ws     *webs.Conn
	// ip string
}
