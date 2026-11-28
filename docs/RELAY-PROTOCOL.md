# The relay protocol, v1

A relay lets a phone reach i-mux's phone server (D104) from anywhere, through a server the user
runs (D115). i-mux's computer side (`imux phone relay`, `imux __phone-serve`) and `imux-relay`
([i-mux-relay](https://github.com/hamzayogurtcuoglu/i-mux-relay)) both follow this document;
neither imports the other's code. A change that an older relay or an older i-mux would not
understand is a new version (v2), never a change to v1.

## Parts

- **The computer:** i-mux's phone server. It dials the relay and keeps the connection.
- **The relay:** forwards HTTP requests from phones to the computer of a room. It opens no message
  and knows nothing of i-mux beyond this document.
- **The phone:** a browser. It talks plain HTTPS to the relay and never knows a relay is there.
- **A room** is one computer's address on a relay: 16 random bytes in URL-safe base64 without
  padding (22 characters, `[A-Za-z0-9_-]{22}`), made by the computer when the relay is set up and
  kept with it.
- **The secret** is the relay's: 32 random bytes in URL-safe base64 without padding (43
  characters), made on the relay's first start. Only holders of it may connect a computer. The
  relay compares it in constant time.
- **The link** is how a user hands a relay to i-mux: `https://<host>[:<port>]#<secret>`. It is
  pasted into i-mux, never opened: the secret stays in the fragment, which no browser sends.

## Paths on the relay

| Path | Who | What |
|---|---|---|
| `GET /` | anyone | a one-line page saying this is an i-mux relay |
| `GET /healthz` | anyone | `200 ok` |
| `GET /v1/check` | the computer | `204` with the right secret, `401` without it |
| `GET /v1/connect?room=R` | the computer | the computer's connection (below) |
| `/r/R` | a phone | `308` to `/r/R/` |
| `/r/R/<path>` | a phone | forwarded to the computer of room `R` as `/<path>` |

Anything else is `404`.

## The computer's connection

The computer sends, over HTTPS with HTTP/1.1 (it must not negotiate HTTP/2 with the relay for
this request), through `HTTPS_PROXY` when set:

```
GET /v1/connect?room=R HTTP/1.1
Host: relay.example.com
Authorization: Bearer <secret>
Connection: Upgrade
Upgrade: imux-relay.v1
User-Agent: imux/<version>
```

The relay answers:

- `101 Switching Protocols` with `Upgrade: imux-relay.v1`: the connection is now the room's.
- `400` the room is not well formed; `401` the secret is wrong; `426` the `Upgrade` is missing or
  names a version the relay does not speak (its answer's `Upgrade` lists the ones it does);
  `429` the relay has as many rooms as it allows.

After `101`, on that same connection, **the relay is an HTTP/2 client and the computer an
HTTP/2 server**, with prior knowledge (no TLS inside, no `h2c` upgrade: the relay sends the
HTTP/2 preface first). The computer serves the same handler it serves on the LAN.

- One connection per room. A new one for a room that has one replaces it: the relay closes the
  older one (a computer that woke from sleep is the usual case).
- The relay sends an HTTP/2 PING when nothing came from the computer for 30 s, and drops a
  connection that does not answer within 15 s. The computer, on any loss, dials again after 1 s, doubling up to 60 s, back to 1 s after
  a connection that lasted a minute.
- A `401` or `426` stops the computer dialing; it reports it (the Phone popup, `imux phone
  relay`) until the relay is set up again.

## Forwarding

A request to `/r/R/<path>?<query>` goes to room `R`'s computer as `/<path>?<query>`.

- Methods: `GET`, `HEAD`, `POST`; others get `405`.
- Request bodies: at most 64 KiB (`413` otherwise).
- Request headers passed on: `Accept`, `Accept-Language`, `Content-Type`, `Content-Length`,
  `Last-Event-ID`, `User-Agent`. Every other one (cookies and `Authorization` among them) is
  dropped. The relay adds `X-Imux-Relay: v1`.
- Responses are passed on as they come, headers and all, without buffering (the event stream
  is Server-Sent Events). The relay adds `Strict-Transport-Security: max-age=31536000` when it
  serves TLS itself.
- No computer connected for `R` (or `R` not well formed): `503` (`404` for a malformed room)
  with a short page: "This computer is not connected to the relay."
- Limits (the relay's flags change them): 100 rooms; 100 requests at once per room; per client
  address 20 requests a second with bursts of 40 (`429` beyond); 10 s to send request headers.
  Streams have no time limit while both ends are there.

## What a relay can see

Every message between a paired phone and the computer is sealed with the device's key (D104),
which only the two hold; the relay passes ciphertext. It can still see the addresses and times
of the phone and the computer, the sizes of what passes, the room, the device ids in requests,
and a pairing token (valid once for two minutes, useless without the key in the QR code's
fragment, which no browser sends). It also passes the app's own files (HTML, JavaScript, CSS),
which it could change: **use a relay you run or trust.**

## Versions

The version is in the `Upgrade` token and the paths (`/v1/…`). A relay may speak several; a
computer asks for the newest it speaks and, on `426`, the newest the relay lists that it also
speaks. v1 is the only one.
