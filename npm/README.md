# oi-proxy CLI

Lightweight HTTP reverse proxy for local development: route third‑party domains through a local port, useful when cookie-based auth is involved.

Install
```sh
npm install oi-proxy
```

Usage
```sh
# minimal
npx oi-proxy --port 8080 --target https://api.github.com

# with extra options
npx  oi-proxy --host 0.0.0.0 --port 8080 --target https://api.github.com --strip-prefix /api --cookie-domain api.local
```

or inside package.json
```json
{
  "scripts": {
    "auth": "oi-proxy --port 4433 --target https://auth.mysite.com",
    "api": "oi-proxy --port 8080 --target https://api.mysite.com --strip-prefix /api"
  }
}
```

## CLI flags

| Flag | Description | Default |
|------|-------------|---------|
| `--host` | Interface to bind | `localhost` |
| `--port` | HTTP port | `80` |
| `--target` | **Required** target base URL | — |
| `--cookie-domain` | Overrides `Domain` in `Set-Cookie` | `host` |
| `--strip-prefix` | Removes the given prefix from request paths | `""` |
| `--insecure` | Disable TLS verification to the target | `false` |
| `--cors-allow-origin` | Override `Access-Control-Allow-Origin` | `""` - falls back to request Origin/Referer|
| `--cors-allow-headers` | Override `Access-Control-Allow-Headers` | `Content-Type, Authorization, X-Requested-With` |
| `--cors-allow-methods` | Override `Access-Control-Allow-Methods` | `GET,POST,PUT,PATCH,DELETE,OPTIONS` |
| `--replace-location` | Replace domain in `Location` header: `"old:new"`. If old empty, uses target host; if new empty, uses `host:port`. Example `":"` replaces target host with local host:port | `""` |

Packages:

- `npm` — main CLI wrapper with `optionalDependencies`.
- `npm-platforms/*` — platform-specific packages (`@oi-proxy/proxy-darwin-arm64`, `...`) each shipping a single binary.
