## Shephard API Skill

You can use the following APIs to manage your swarm. The base **URL** for the swarm is: `http://localhost:%d`.

---

### 1. Spawn a Job & Poll for Status

To query any API, you will need the slug, query parameters, and the body. Use the following endpoint to initiate a task:

* **Endpoint:** `POST /app/{slug}`
* **Body:** Any JSON or plain text — passed as `body` to the agent's prompt template.
* **Response (202):** `{"id": "req_...", "next_step": "..."}`

#### Example: Basic API Call

```bash
curl -s -X POST $URL/app/{slug}?param1=value1&param2=value2 \
    -H 'Content-Type: application/json' \
    -d '{body}'

```

#### Example: Sending Images

You can send images in base64 format by adding the `x-swarm-images-key` to the header:

```bash
curl -s -X POST $URL/app/{slug}?param1=value1&param2=value2 \
    -H 'Content-Type: application/json' \
    -H 'x-swarm-images-key: images' \
    -d '{body}'

```

---

### 2. Polling for Status

Once you have received the request ID, you can poll the status of the job.

* **Endpoint:** `GET /status/{id}`
* **Response:** `{"job": {...}, "metrics": {...}, "done": true/false, "stdout": <model_response>}`

#### Example: Manual Poll

```bash
curl -s $URL/status/req_abc123

```

#### Example: Composite CLI (Spawn + Poll)

This single command spawns a job and polls every 2 seconds until completion:

```bash
curl -s -X POST $URL/app/{slug}?param1=value1&param2=value2 \
    -H 'Content-Type: application/json' \
    -d '{body}' | \
jq -r .id | \
xargs -I{} bash -c \
'until curl -s $URL/status/{} | \
    jq -e ".done == true" > /dev/null; \
    do sleep 2; done; curl -s $URL/status/{} | \
    jq .stdout'

```

---

### 3. Scheduled Jobs (Cron)

To view currently scheduled jobs, use the following:

* **Endpoint:** `GET /crons`
* **Example:**
```bash
curl -s $URL/crons

```



---

### 4. Terminate Jobs

To "pull the plug" on a specific task:

* **Endpoint:** `POST /kill`
* **Body:** `{"id": "req_..."}`
* **Example:**
```bash
curl -s -X POST $URL/kill \
  -H 'Content-Type: application/json' \
  -d '{"id": "req_abc123"}'

```



---

### 5. Shephard Agent API

Shephard is an AI agent with a deeper understanding of the system and stateful information. It is recommended for most management tasks.

* **Endpoint:** `POST /shephard`
* **Body:** `{"message": string}`

#### Natural Language Prompt

The API accepts natural language which is executed by a coding agent.

```bash
curl -s -X POST $URL/shephard \
    -H 'Content-Type: application/json' \
    -d '{"message": "What is common pattern in all jobs by yash?"}' | \
jq -r .id | \
xargs -I{} bash -c \
   'until curl -s $URL/status/{} | \
    jq -e ".done == true" > /dev/null; \
    do sleep 2; done; curl -s $URL/status/{} | \
    jq .job.Output'

```

#### Direct Bash Execution

You can run bash commands directly by starting the `message` with an exclamation mark (`!`).

```bash
curl -s -X POST http://localhost:%d/shephard \
  -H 'Content-Type: application/json' \
  -d '{"message": "!ls -l"}' | \
jq -r .id | \
xargs -I{} bash -c \
   'until curl -s $URL/status/{} | \
    jq -e ".done == true" > /dev/null; \
    do sleep 2; done; curl -s $URL/status/{} | \
    jq .job.Output'

```

---

### Notes

* **Background Polling:** Always poll `/status/{id}` in a background loop and provide succinct updates to the user as the status changes.
* **Persistence:** Jobs remain queryable even after completion.
* **Best Practice:** Use the composite CLI call for spawning and polling to ensure a seamless workflow.
