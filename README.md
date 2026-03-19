# Swarm Buddy (& friends)

https://yashbonde.com/swb

```bash
git clone --depth 1 https://github.com/yashbonde/swarm-buddy.git
cd swarm-buddy

# set the env var (only works with gemini rn.)
export GEMINI_TOKEN="<your-gemini-token>"

# Run the first example (single prompt)
go run examples/main.go --block

# Run the second example (multi step conversation)
go run examples/main.go --sequence --show-history

# Run the third example (with logs for undertsanding hooks)
go run examples/main.go --block --logs
```
