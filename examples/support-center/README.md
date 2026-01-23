# Support Center Example

This example demonstrates how to build a customer support center using VettID Service Vault's call capabilities.

## Features

- Voice and video calls to VettID users
- Agent management and availability tracking
- Webhook handling for call events
- WebRTC signaling integration points

## Running

```bash
go run main.go
```

The server starts on port 3000.

## API Endpoints

### Initiate a Call

```bash
POST /api/calls
Content-Type: application/json

{
  "user_id": "user123",
  "type": "video",        // "voice" or "video"
  "purpose": "Technical support",
  "agent_id": "agent-001" // optional, auto-assigns if not specified
}
```

### List Agents

```bash
GET /api/agents
```

### List Active Calls

```bash
GET /api/calls
```

## Webhook Events

Configure your VettID Service Vault to send webhooks to:

```
POST /webhooks/vettid/call
```

Events handled:
- `call.accepted` - User accepted the call
- `call.rejected` - User rejected the call
- `call.missed` - Call timed out
- `call.ended` - Call ended

## Integration Notes

In a production environment, you would:

1. **WebRTC Media Server**: Integrate with a media server (Janus, mediasoup, Twilio) to handle the actual audio/video streams.

2. **Agent Dashboard**: Build a web UI for agents to:
   - See incoming calls
   - Provide their SDP answer
   - Exchange ICE candidates

3. **Queue Management**: Implement call queuing when all agents are busy.

4. **Recording**: Add call recording for quality assurance.

## Call Flow

```
1. Support Center initiates call
   POST /api/v1/call/initiate -> VettID Service Vault

2. Vault sends call request to user's VettID app
   MessageSpace -> User's device

3. User accepts/rejects
   - Accept: Returns SDP answer, ICE begins
   - Reject: Returns rejection reason

4. Webhook notification
   VettID -> POST /webhooks/vettid/call

5. WebRTC connection established
   Agent browser <-> Media Server <-> User app
```
