# meshora

Live map and packet analyzer for the [MeshCore](https://meshcore.co.uk/) mesh
around Greater Boston.

It reads a public packet feed, decodes the MeshCore wire format, matches each
observed path against known node locations, and draws the packet flows on a map
as they come in. There's also a packet history table with filters, a per-node
view (role, path-hash size, recent forwarding counts), and a timeline you can
scrub back through.

## Building

The frontend is embedded into the Go binary, so build it first:

```
npm --prefix web ci
npm --prefix web run build
go build ./cmd/meshora
```

## Running

```
./meshora
```

Then open http://localhost:8080.

By default it streams from the analyzer WebSocket feed and seeds the node and
observer list once from the analyzer's REST API on startup.

Flags:

```
-addr       listen address (default :8080)
-db         sqlite path (default meshora.db)
-source     feed or broker (default feed)
-feed-url   analyzer feed url (default wss://analyzer.bostonme.sh/)
-bootstrap  seed nodes from the REST API on startup (default true)
-web        serve the frontend from a directory instead of the embed
```

The broker source reads packets straight from MQTT instead, and takes `-broker`,
`-user`, `-pass`, and `-topic`.

## Dev

`npm --prefix web run dev` runs Vite with HMR and proxies `/api` and `/ws` to a
backend on `:8080`, so run `./meshora` next to it.
