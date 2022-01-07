# XPeer Protocol Spec

**Create pseudo Peer-To-Peer anywhere!**

XPeer is trying to achieve a P2P feeling for developers even in environments where it isn't possible, eg. Browsers.
XPeer makes pseudo P2P possible with the help of a *Relay-Server* and websocket connections between the relay server and it's clients (the "peers").

Every Relay-Server should expose a endpoint /xpeer which allows websocket connections to be established.

XPeer introduces a concept so called VPeer (virtual peer). A VPeer has a unique ID like every other peer, but instead of being connected to the server via websocket, it is a virtual construct on the server. Normal peers can subscribe to VPeers and broadcast messages to all other subscribers. A VPeer also consists of a shared state, which notifies all subscribers when modified.


## How it works:

1. A client send an operation to the server
2. The server processes the operation
3. The server sends a message (based of the operation) to one ore more clients

## Formats

### Operation 

{operationCode}::{targetID}::{payload}

- operationCode: 8 Character unique code (see [Operations](#operations))
- targetID: a connection ID
- payload: whatever you want encoded into a string

#### Possible Operation Codes

| Code     | Operation                     |
| -------- | ----------------------------- |
| sendPing | send a ping request           |
| sendPong | answer to a ping request      |
| crtVPeer | create a new vpeer            |
| delVPeer | delete a vpeer                |
| conVPeer | connect to a vpeer            |
| disVPeer | disconnect from a vpeer       |
| sendPeer | send a message to a peer      |
| putState | override the state of a vpeer |
| patState | update the state of a vpeer   |

### Message 

{messageCode}::{senderID}::{payload}

- messageCode: 8 Character unique code (see [Messages](#messages))
- senderID: a connection ID
- payload: whatever you want encoded into a string

#### Possible Message Codes

| Code     | Message                                                                |
| -------- | ---------------------------------------------------------------------- |
| oprResOk | operation executed successfully                                        |
| recvPeer | received message from peer                                             |
| sendPing | peer requested a ping response                                         |
| sendPong | peer responded to a ping (with a pong)                                 |
| gPeerCId | message contains a peer id                                             |
| errorMsg | an error happened while executing the operation [see #errors](#errors) |
| stateMut | the state of a vpeer changed                                           |


### Connection ID

A connection ID is a 22 Character unique string. It consists of a 16 Character ID, a "@" Symbol and a 5 Character unique server ID.

## Operations

### sendPing

Ping a connection ID;

> Target: the peers ID

> Server answers with success or error;

### sendPong

Answer to a ping request;

> Target: the peers ID

> Server sends no answer;

### crtVPeer

Create a virtual peer with a shared state, that can be connected from multiple other peers;

> Target: the peer ID, which should get the answer with the new vpeers ID;

> Server answers with the vpeers ID or an error;

### delVPeer

Delete a virtual peer, automatically disconnects all connected peers;

> Target: the vpeers ID

> Server sends no answer;

### conVPeer

Connect to a virtual peer to listen to messages or state updates;

> Target: the vpeers ID

> Server answers with teh state of the vpeer (success)  or error;

### disVPeer

Disconnect from a virtual peer;

> Target: the vpeers ID

> Server answers with success or error;

### sendPeer

Send a messages directly to a peer (can be a vpeer, which acts like a broadcaster in this situation)

> Target: the peers ID

> Server answers with success or error;

### putState

Override the shared state of a vpeer;

> Target: the vpeers ID

> Server answers with success or error;

### patState

Update the shared state of a vpeer (works like object merging in JS);

> Target: the vpeers ID

> Server answers with success or error;

## Messages 

### oprResOk

Indicates, that a operation was successful;

> Sender: own ID;

> Payload: receivers ID;

### recvPeer

A peer send a message to you;

> Sender: the peers ID;

> Payload: the message;

### sendPing

Requests a ping response;

> Sender: the peers ID, which wants the response;

> Payload: none;

### sendPong

Indicates a successful ping operation;

> Sender: the peers ID, which sent the answer;

> Payload: none;

### gPeerCId

Sends a connection ID. It is yous to make you aware of your own ID, or IDs of vpeers;

> **If** the sender equals the payload: it's your id;

> **If** the senders equals your ID and the payload doesn't match: the payload is the ID of a new vpeer;

### errorMsg

Indicates, that a error happened.

> Sender: your ID;

> Payload the error message;


### stateMut

Indicates a state update of a connected vpeer;

> Sender: the vpeers ID;

> Payload the new state encoded as json

## Errors

### error: message too short

The message you sent is shorter than the minium message length. This likely means, that either you operationCode or you Id is not valid / too short 

### error: invalid message format

The message you send is in an invalid format. In most cases the dividers (`::`) are in wrong positions.

### error: message operation is unknown

The operation of your message is not known.

### error: target could not be located

The target of you operation is not registered on the server

### error: that target peer is not virtual

The target of the operation has to be virtual, but isn't.

### error: the state string is formatted invalidly

The state is not formatted as valid json.


## Limitations

- VPeers can't connect to VPeers
- A VPeers state <u>**has to be**</u> expressed as JSON
- XPeer currently only works with one server