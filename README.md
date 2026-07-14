# wol-proxy


## Sample Network Traffic Flow

### Client Connects: 

A client (e.g., `Msty Studio`) connects to the local proxy running on your Windows 11 machine at `localhost:11434`.

### Data from Client: 

The client sends data to the proxy.

### Server Check: 

The proxy checks if the server (`192.168.1.166:11434`) is awake. If the server is sleeping, it holds the packet and sends a Wake-on-LAN magic packet to wake up the server (assuming the server BIOS/UEFI settings allow WOL magic packets on its network interface). If the server is awake, it proceeds without holding any packets.

### Server Response: 

After the server wakes up (or if it was already awake), it starts processing requests.

### Bi-Directional Data Flow:

A goroutine copies data from the client to the server on an ongoing basis. The main execution thread within `proxyData` simultaneously copies data from the server back to the client.


# Build

### Step 1: Build as a non-console application.

```bash
go build -ldflags "-H=windowsgui" -o wol-proxy.exe wol-proxy.go
```

The `-ldflags "-H=windowsgui"` flag tells Windows not to attach a console window to the executable at runtime.

### Step 2: Set it to Run on Startup

Press Win + R to open the Windows Run dialog.

Type `shell:startup` and press Enter. This opens your user account's Startup folder.

Right-click inside this folder, select `New > Shortcut`.

Click Browse and locate your newly compiled `wol-proxy.exe`.

Click Next, name the shortcut (e.g., "Wake on LAN Proxy"), and click Finish.

### Debug

To run it in debug mode, add a `-d` command line flag (e.g., `...\wol-proxy.exe -d`).