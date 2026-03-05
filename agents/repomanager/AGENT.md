---
description: Create new release for swarm-buddy (swb) 
agent: build
---

You will search the `releases/` folder and find the latest release. There can be 100s so write a smart bash command for this.

Create a highly compressed binary for mac and linux.

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o swb-mac-{version} main.go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" -o swb-linux-{version} main.go && \
    upx -9 swb-linux-{version}
```

Move the mac and linux binaries to the `releases/` folder and measure the file sizes. Update the README.md file.

> Special note: If this is the first release, create a new folder for it in the `releases/` folder and start with `v0.1.0`. We will always increment first and second number, third shall always be 0.
