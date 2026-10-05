# longshift-market-mcp

Example [MCP](https://modelcontextprotocol.io/) server in Go that wraps
[Frankfurter](https://www.frankfurter.app/) — a free, open-source FX API with
no API key.

The same binary runs over stdio for a local MCP client, or over streamable HTTP
so a Longshift data cell can host it as a tenant-scoped Firecracker app. This
repository also publishes the guest image the cell consumes:

`ghcr.io/ecociel/longshift-market-mcp`

## Why Frankfurter

Checked 2026-10-05 against the live public API:

| Check | Result |
| --- | --- |
| Key | None. `https://api.frankfurter.dev/v2/rates` and `https://api.frankfurter.app/latest` both answer without auth. |
| Cost | Public instance is free; no monthly quota. Docs: rate-limited against abuse only. |
| License / source | Open source ([lineofflight/frankfurter](https://github.com/lineofflight/frankfurter)). |
| Stability | v1 (`api.frankfurter.app`) is deprecated but kept indefinitely. This server uses current **v2** at `api.frankfurter.dev`. ECB official reference rates are `/v2/providers/ecb/...` (ECB series since 1999-01-04). |

Alpha Vantage, Finnhub, and FRED all need keys. Yahoo unofficial scrapes are not a stable contract. Frankfurter is the better fit.

Default provider for the rate tools is **ECB**. Pass `provider: "all"` on a tool call (or set `MARKET_MCP_PROVIDER=all`) for Frankfurter’s blended multi-bank feed.

## MCP tools

| Tool | Upstream |
| --- | --- |
| `list_currencies` | `GET /v2/currencies` |
| `list_providers` | `GET /v2/providers` |
| `get_rates` | `GET /v2/providers/{provider}/rates` (or `/v2/rates` when blended) |
| `get_rate` | `GET /v2/providers/{provider}/rate/{base}/{quote}` |
| `convert` | same single-pair endpoint; this server multiplies `amount * rate` |

`convert` exists because v2 has no conversion route — the docs tell clients to apply the rate themselves.

## Configure and run

No secrets. Configuration is optional environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `MARKET_MCP_TRANSPORT` | `http` | `http` (streamable HTTP) or `stdio` |
| `MARKET_MCP_ADDR` | `:8080` | Listen address in HTTP mode. Bind `0.0.0.0`, not localhost, inside a cell. |
| `MARKET_MCP_BASE_URL` | `https://api.frankfurter.dev` | Frankfurter origin. Override for a self-hosted instance. |
| `MARKET_MCP_PROVIDER` | `ecb` | Default rate provider. `all` / `blended` uses the public blended feed. |
| `MARKET_MCP_HTTP_TIMEOUT` | `15s` | Upstream HTTP timeout. |
| `MARKET_MCP_USER_AGENT` | `longshift-market-mcp/0.1` | Sent on Frankfurter requests. |

HTTP mode also serves `GET /healthz` (`ok`) and `GET /` (service JSON). The MCP endpoint is `/mcp`.

```bash
go test ./...
go run ./cmd/market-mcp
# MCP:  http://127.0.0.1:8080/mcp
# health: http://127.0.0.1:8080/healthz
```

Local stdio (Cursor / Claude Desktop and similar):

```json
{
  "mcpServers": {
    "market": {
      "command": "go",
      "args": ["run", "./cmd/market-mcp"],
      "env": {
        "MARKET_MCP_TRANSPORT": "stdio"
      }
    }
  }
}
```

Point a streamable-HTTP MCP client at `http://<host>:8080/mcp` after HTTP mode is up.

## OCI image this repo publishes

GitHub Actions workflow [`.github/workflows/publish-image.yml`](.github/workflows/publish-image.yml)
builds `linux/amd64` and pushes to GHCR:

| Tag | When |
| --- | --- |
| `ghcr.io/ecociel/longshift-market-mcp:latest` | default branch |
| `ghcr.io/ecociel/longshift-market-mcp:sha-<full-sha>` | every build |
| `ghcr.io/ecociel/longshift-market-mcp:<short-sha>` | every build |
| `ghcr.io/ecociel/longshift-market-mcp:pr-<n>` | pull requests |
| `ghcr.io/ecociel/longshift-market-mcp:<x.y.z>` | `v*` tags |

The image is a static Go binary on `gcr.io/distroless/static-debian12:nonroot`.
`ENTRYPOINT` is `/market-mcp`. It listens on `8080`. There is no kernel, no
init system, and no secret in the layers.

Local build (optional; CI is the source of the published name):

```bash
docker build -t ghcr.io/ecociel/longshift-market-mcp:dev .
docker run --rm -p 8080:8080 ghcr.io/ecociel/longshift-market-mcp:dev
```

First GHCR push creates a package that is private to the org by default. For a
data cell to pull it, either make
[the package public](https://docs.github.com/en/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility)
or grant the cell’s pull identity `read:packages`. Do not put a GHCR token in
the guest image or in guest env.

## Deploy that image as a Longshift data-cell app

### Source of the contract

The task points at current `main` of `https://github.com/ecociel/longshift`,
especially `docs/19-cell-apps.md` and merged PR **#258** (`dde2427`, “Deploy an
OCI image onto a data cell”).

This environment cannot read that repository (GitHub returns 404 / “not found”
for anonymous and for the tokens available here). The steps below therefore
follow the **deploy contract those sources are specified to encode**, not an
invented CLI. If you have the repo locally and `docs/19-cell-apps.md` and the
`dde2427` code disagree, **follow the code on `main`**.

### Contract (what the cell accepts)

| You supply | You do not supply |
| --- | --- |
| One OCI image reference | A kernel / `vmlinux` |
| Tenant that owns the app | A rootfs disk (`rootfs.ext4`, `overlay.img`, …) |
| Non-secret runtime config the control plane already supports | Guest-visible secrets, tokens, or `.env` files |

The cell:

1. Pulls the OCI image.
2. Materializes a guest rootfs **internally** from the image layers.
3. Boots **Firecracker only**, with the **platform kernel**.
4. Starts the image `ENTRYPOINT` (`/market-mcp`) as a **tenant-scoped app**.
5. Keeps **secrets off the guest**. Registry credentials, cell identity, and any
   later secret store stay on the host / control plane.

This image matches that split: userspace only, `linux/amd64`, process as PID 1,
port 8080, no key material.

### Packaging for Longshift (not a generic container host)

- Do not wrap the image in Docker Compose, `runc`, Kata, or a second guest
  runtime. The cell is Firecracker; the OCI image is input, not the hypervisor.
- Do not bake a kernel or an ext4 rootfs next to the Dockerfile. If a deploy
  form has “kernel” or “rootfs” fields, leave them to the platform.
- Do not `docker save` a rootfs tarball as the artifact. Push the image to
  GHCR; the cell consumes the registry reference.
- Do not put `MARKET_MCP_*` secrets in the image. This server has none. If you
  later add authenticated upstreams, inject those values from the cell control
  plane so they never land in the image or in a guest-writable secret file.
- The guest needs **egress** to `api.frankfurter.dev` (or your
  `MARKET_MCP_BASE_URL`). It does not need inbound credentials.

### Cell app steps

Use whatever app-create path current `main` exposes after #258 (API, CLI, or
UI). The payload that matters is:

1. **Image** (required): `ghcr.io/ecociel/longshift-market-mcp:latest`
   or pin `ghcr.io/ecociel/longshift-market-mcp:sha-<git-sha>` from the
   workflow that built the revision you want.
2. **Tenant**: the tenant that should own the app. The app is not a
   cell-global service.
3. **Guest listen port**: `8080`. MCP clients use path `/mcp`. Liveness can
   probe `/healthz` from wherever the platform runs health checks — that probe
   belongs on the host side if the platform offers one.
4. **Optional env** (non-secret): `MARKET_MCP_PROVIDER`, `MARKET_MCP_BASE_URL`,
   `MARKET_MCP_ADDR=:8080`. Omit anything that looks like a token.
5. **Registry pull**: host-side. If GHCR is private, configure pull auth on
   the cell, not as `GITHUB_TOKEN` inside the guest.

After the app is running, point an MCP client at the URL the cell publishes
for that app, path `/mcp`.

### If you are reading longshift `main`

Confirm these against the #258 implementation, not only the doc:

- Image field is an OCI reference; there is no caller-supplied kernel/rootfs.
- Runtime enum / path is Firecracker; other isolators are rejected.
- App records include a tenant id and cannot be listed across tenants.
- Secret / registry credentials are not copied into the guest filesystem.

Where the doc and that code diverge, change the deploy call to match the code
and treat the doc as stale.

## Layout

```
cmd/market-mcp/          HTTP or stdio process
internal/config/         environment
internal/frankfurter/    v2 HTTP client
internal/server/         MCP tools + /mcp + /healthz
Dockerfile               linux/amd64 guest image
.github/workflows/       test, then push to GHCR
```
