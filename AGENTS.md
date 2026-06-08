# Remote Agents Setup Guide

This guide explains how to connect your remote environments (Agents) securely to the main Onet server so they can receive internet traffic from your custom subdomains.

## How it works

The main Onet server acts as the ingress controller. It automatically forwards traffic entering `*.oggree.com` through Cloudflare down to the embedded FRP (Fast Reverse Proxy) server.

To route a specific subdomain (like `app1.oggree.com`) to a specific remote machine, you simply need to run the `frpc` (FRP Client) on that remote machine. 

## Step 1: Download the Client

On the remote machine that hosts your application, download the official `frpc` binary.
You can find the latest releases on the official GitHub page:
[https://github.com/fatedier/frp/releases](https://github.com/fatedier/frp/releases)

*Example for Linux (amd64):*
```bash
wget https://github.com/fatedier/frp/releases/download/v0.61.0/frp_0.61.0_linux_amd64.tar.gz
tar -zxvf frp_0.61.0_linux_amd64.tar.gz
cd frp_0.61.0_linux_amd64
```

## Step 2: Configure the Client

Create a configuration file named `frpc.toml` on your remote machine.

You will need three pieces of information from the main Onet server's `/etc/onet.yaml`:
- **Server IP**: The public IP address of the machine running Onet (or a domain that points to it). If Onet is running locally for testing, it will be `127.0.0.1`.
- **Server Port**: The port your remote agents connect to, found under `frp_bind_port` in `onet.yaml` (defaults to `7000`).
- **FRP Auth Token**: Found under `frp_auth_token` in `onet.yaml`.

*Example `frpc.toml`:*

```toml
# frpc.toml

serverAddr = "IP_ADDRESS_OF_ONET_SERVER"
serverPort = 7000

auth.method = "token"
auth.token = "change-this-token" # Must match frp_auth_token on the Onet server

# Define the local service you want to expose
[[proxies]]
name = "my-web-app"
type = "http"
localIP = "127.0.0.1"
localPort = 3000                 # The local port your app is running on
customDomains = ["app1.oggree.com"] # The public subdomain you want mapped to this app
```

## Step 3: Start the Agent

Run the `frpc` binary using your configuration file:

```bash
./frpc -c ./frpc.toml
```

If successful, you will see logs indicating that the client has connected to the server.
Now, whenever a user visits `http://app1.oggree.com`, Cloudflare will route the traffic to Onet, and Onet will instantly tunnel that traffic securely down to your remote machine on port `3000`!

## Adding More Apps

You can run multiple proxy blocks in a single `frpc.toml` to expose different apps on different ports with different subdomains. Just add another `[[proxies]]` block. 
You can also run `frpc` on as many different remote machines as you like. They will all seamlessly connect back to Onet.
