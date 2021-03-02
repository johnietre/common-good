const maxCoins = 5;
var room;
var displayName;

var ws = new WebSocket("ws://192.168.1.125:8000/socket/", [], true);
ws.onopen = function() {
  console.log("opened");
};
ws.onerror = function(err) {
  console.log(err);
  window.alert("Error");
  lobbyDiv.hidden = true;
  pregameDiv.hidden = true;
  depositDiv.hidden = true;
  connectDiv.hidden = false;
  window.location.reload();
};
ws.onmessage = function(msg) {
  msg = JSON.parse(msg.data);
  console.log(msg);
  if (msg.action == "error") {
    if (msg.action == "room doesn't exist") {
      connectErrP.innerHTML = "Room doesn't exist";
    } else if (msg.action == "room full") {
      pregameDiv.hidden = true;
      connectDiv.hidden = false;
      window.alert("Room already full");
    } else if (msg.action == "member already exists") {
      pregameErrP.innerHTML = "Name taken";
    }
  } else if (msg.action == "created") {
    connectDiv.hidden = true;
    pregameDiv.hidden = false;
    pregameH1.innerHTML = "Room ID: " + msg.contents;
  } else if (msg.action == "joined") {
    connectDiv.hidden = true;
    pregameDiv.hidden = false;
    pregameH1.innerHTML = "Room ID: " + msg.contents;
  } else if (msg.action == "added") {
    // pregameDiv.hidden = true;
    // lobbyDiv.hidden = false;
    console.log("added");
    pregameErrP.value = "Added";
  } else if (msg.action == "started") {
    pregameDiv.hidden = true;
    lobbyDiv.hidden = false;
  } else if (msg.action == "ended") {
    ws.close();
    lobbyDiv.hidden = true;
    connectDiv.hidden = false;
  }
};

/* Connect */

var connectDiv = document.getElementById("connect-div");
var roomInput = document.getElementById("room-input");
var connectErrP = document.getElementById("connect-err-p");
function joinRoom() {
  var r = roomInput.value;
  room = r;
  ws.send(r);
}

function startRoom() {
  ws.send("create");
}

/* Pregame */

var pregameDiv = document.getElementById("pregame-div");
var pregameH1 = document.getElementById("pregame-h1");
var nameInput = document.getElementById("name-input");
var membersList = document.getElementById("members-list");
var pregameErrP = document.getElementById("pregame-err-p");
function submitName() {
  pregameErrP.value = "";
  displayName = nameInput.value;
  ws.send(displayName);
}

/* Lobby */

var lobbyDiv = document.getElementById("lobby-div");
var lobbyTimeSpan = document.getElementById("lobby-time-span");

/* Deposit */

var depositDiv = document.getElementById("deposit-div");
var personalInput = documet.getElementById("personal-input");
var taxInput = document.getElementById("tax-input");
var coinsSpan = document.getElementById("coins-span");
// var depositTimeSpan = documet.getElementById("deposit-time-span");
// var depositErrP = document.getElementById("deposit-err-p");
var personal = 0;
var tax = 0;

function depositCoins(elem) {
  // depositErrP.innerHTML = "";
  var coins = Number.parseInt(elem.value);
  if (elem.id == "personal-input") {
    if (coins + tax > maxCoins) {
      // depositErrP.innerHTML = "Not enough coins";
      elem.value = personal;
    } else {
      personal = coins;
      coinsSpan.innerHTML = maxCoins - personal - tax;
    }
  } else {
    if (coins + personal > maxCoins) {
      // depositErrP.innerHTML = "Not enough coins";
      elem.value = tax;
    } else {
      tax = coins;
      coinsSpan.innerHTML = maxCoins - personal - tax;
    }
  }
}
