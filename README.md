# savalet

Savalet (server valet) is an HTTP front door to a fixed set of local commands. It lets other systems invoke single purpose binaries on a host over HTTP instead of SSH, with the set of possible invocations closed at configuration time.

The authorization model is three properties:

- Clients name an action and pick parameter values from declared enums. They never send an executable name, an argument vector, or a timeout.
- argv is assembled server side by slotting values into fixed positions. No shell is involved at any point, and a value can never alter the structure of a command.
- Everything that can be checked statically is rejected at startup. Request handling reduces to map lookups.

## Motivation

SSH based system integration accumulates debt around key management: lost keys, keys of departed operators, and no version control over who can run what. Savalet replaces the "SSH in and run a command" pattern with an HTTP endpoint whose entire capability set is a reviewable configuration file.

## Installation

Download a binary from [Releases](https://github.com/zinrai/savalet/releases).

## Configuration

A complete annotated example lives at [`example/savalet.yaml`](./example/savalet.yaml).

```yaml
actions:
  - name: restart-service
    description: "Restart a managed service"
    argv: ["/usr/bin/systemctl", "restart", "{service}"]
    params:
      service:
        enum: [nginx, postgresql]
    timeout: 60
```

| Field | Required | Default | Meaning |
|---|---|---|---|
| `listen` | no | `unix:/run/savalet/savalet.sock` | `unix:` prefix for a Unix domain socket, otherwise a TCP address |
| `max_output_bytes` | no | 1048576 | Per stream cap on captured stdout and stderr |
| `max_timeout` | no | 60 | Upper bound for every action's `timeout`, in seconds |
| `identity_header` | no | unset | Header recorded in the audit log, see Audit log |
| `actions[].name` | yes | | URL path segment, `^[a-z0-9][a-z0-9-]*$` |
| `actions[].argv` | yes | | Argument vector, `argv[0]` must be an absolute path to an executable |
| `actions[].params` | no | | Parameter names (`^[a-z][a-z0-9_]*$`) mapped to `enum` value lists |
| `actions[].timeout` | yes | | Seconds, between 1 and `max_timeout` |

### Rules the config must satisfy

Startup and `savalet check` reject any configuration that violates these. An unknown key anywhere in the file is also an error.

- `argv` is an argument vector, never a shell string.
- A placeholder is an argv element of exactly the form `{name}`. Partial forms like `--unit={service}` are errors, and a literal brace is inexpressible. A command that needs one gets wrapped in a single purpose binary.
- Each placeholder appears at most once, and every declared parameter must appear in `argv`.
- Enum values must be non-empty, unique, and must not start with `-`.

Two rules cannot be checked statically and are kept by review: the verb belongs in the action name with parameters naming only the targets of that one operation, and a value set that cannot be closed as an enum means writing a single purpose binary for `argv[0]`, not loosening validation.

## Usage

Validate a configuration:

```console
$ savalet check -config /etc/savalet/savalet.yaml
OK: 3 actions
```

Exit codes: 0 for a valid configuration, 1 for an invalid one, 2 when the file could not be read. Because `check` verifies that each `argv[0]` exists and is executable, it is bound to the host and belongs on the deploy target, for example as the `validate` step of an Ansible `template` task.

Run the server:

```console
$ savalet run -config /etc/savalet/savalet.yaml
```

Invoke an action over the socket:

```console
$ curl --unix-socket /run/savalet/savalet.sock \
    -X POST http://savalet/actions/restart-service \
    -d '{"params": {"service": "nginx"}}'
$ curl --unix-socket /run/savalet/savalet.sock http://savalet/healthz
```

## API

| Method | Path | Purpose |
|---|---|---|
| POST | `/actions/{name}` | Execute an action |
| GET | `/healthz` | Liveness check |

The request body is `{"params": {...}}` and may be omitted for actions without parameters. The key set must match the declared parameters exactly. An unknown key is a 400, not silently ignored. Request bodies are limited to 64 KiB.

A response carries every field on every request. No key is ever omitted:

```json
{
  "request_id": "01a2...",
  "action": "restart-service",
  "params": {"service": "nginx"},
  "status": "completed",
  "exit_code": 0,
  "stdout": "",
  "stderr": "",
  "stdout_truncated": false,
  "stderr_truncated": false,
  "started_at": "2026-08-24T04:12:33Z",
  "duration_ms": 1204
}
```

`status` reports whether savalet's machinery worked, and `exit_code` reports what the command said:

| `status` | Meaning | HTTP |
|---|---|---|
| `completed` | The process started and exited on its own. `exit_code` may be non-zero | 200 |
| `timeout` | The action timeout fired and the process was terminated | 200 |
| `signaled` | The process was terminated by a signal for another reason | 200 |
| `spawn_failed` | The process could not be started | 500 |

`exit_code` is `null` when the process did not produce one. `request_id` echoes the `X-Request-Id` header when present, otherwise savalet generates one.

Validation failures return `{"request_id": "...", "error": "..."}` with 400 (bad parameters or body), 404 (unknown action), 405 (wrong method), or 413 (oversized body).

## Execution semantics

- The timeout is an attribute of the action. Clients cannot specify or extend it.
- Execution is detached from the HTTP request. A client disconnect does not interrupt a command that has side effects. The command completes and is audit logged, only the response is lost.
- On timeout, SIGTERM goes to the command's process group, so children of the command are terminated too. After a 5 second grace period the direct child is SIGKILLed. A grandchild that ignores SIGTERM can outlive this, so a command that spawns children should forward termination to them.
- Stopping savalet delivers the same SIGTERM path to in flight commands.
- The child environment is fixed to `PATH` and `LANG=C`. Nothing is inherited from savalet's own environment.
- stdout and stderr are each captured up to `max_output_bytes`, with the overflow discarded and the corresponding `*_truncated` flag set.
- A response is held open for the full run of the command. Anything proxying savalet must be willing to wait `max_timeout` seconds, or long actions lose their responses.

## Audit log

Savalet writes one JSON record per line to stdout, which lands in the journal under systemd:

```json
{"ts":"2026-08-24T04:12:33Z","level":"info","event":"action_executed","request_id":"01a2...","identity":"zinrai","action":"restart-service","params":{"service":"nginx"},"argv":["/usr/bin/systemctl","restart","nginx"],"status":"completed","exit_code":0,"duration_ms":1204}
```

| `event` | When |
|---|---|
| `action_executed` | After every execution, whatever the status |
| `action_rejected` | When validation refuses a request |
| `config_loaded` | At startup, with the config file's SHA-256 and action count |
| `shutdown` | At exit |

The expanded `argv` is logged so a record is readable without the config file version it ran under. In `action_executed`, parameter values are enum members, so no secret can appear in the log. In `action_rejected` the values are arbitrary client input and are truncated to 128 bytes.

`identity_header` names a header whose value is recorded verbatim and unverified, because authentication belongs to the front proxy. The record is trustworthy only while savalet is reachable solely through that proxy, which the Unix domain socket default guarantees. With a TCP listener, the recorded identity is whatever the client chose to send.

## Deployment

Unit files for systemd socket activation are in [`systemd/`](./systemd/). They are the deployment reference, annotated where a directive interacts with action execution.

## Non-goals

- Authentication and TLS. These belong to the front proxy. Savalet must not be directly reachable by untrusted clients.
- Arbitrary commands or argument vectors.
- Pattern or free text parameters, and optional parameters. Both can be added later if a concrete case demands them, the reverse is not possible.
- stdin, environment variables, or working directory control for commands.
- Output streaming, asynchronous jobs, job history. Work longer than `max_timeout` belongs in a job system, not here.
- Concurrency control. Simultaneous requests for the same action run simultaneously, with `TasksMax` as the only backstop.
- An action listing API. The configuration file is the list.
- Reachability. Savalet answers inbound HTTP and does nothing to help the caller reach the host.

## License

This project is licensed under the [MIT License](./LICENSE).
