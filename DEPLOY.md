# Put Sol on the home web server (devnet)

Build on the laptop, carry the files over on a USB stick, and install them at the server's keyboard. There is no SSH step. This server is a machine on the home network. It is not the Qtagg server.

Nginx on that machine is what friends on the internet talk to. Solana stays on devnet. Do not set `ALLOW_MAINNET`.

Stop `go run .` on the laptop once this copy is running. Two backends both try to reveal and resolve the same devnet events.

## What the stick carries

Secrets stay in the files. This document does not repeat them.

| On the laptop | On the stick | On the server | Why |
|---|---|---|---|
| `sol-backend` from `.\build.ps1` | `backend\sol-backend` | `/opt/sol/backend/sol-backend` | The API |
| `frontend\dist\` | `dist\` | `/var/www/sol/` | The pages |
| `.env` | `env` (no leading dot, so Linux shows it) | `/opt/sol/.env` | Helius devnet RPC, both API-Football keys, session secret |
| `docker-compose.yml` | `docker-compose.yml` | `/opt/sol/docker-compose.yml` | MongoDB in Docker, bound to localhost |
| `programs\predictions\keys\deployer-keypair.json` | `keys\` | `/opt/sol/programs/predictions/keys/` | Signs automatic result submission |
| `council-signer.json` | `keys\` | same folder | Second result signer |
| `reveal-keeper.json` | `keys\` | same folder | Publishes coupon reveals |

Leave the other files in `programs\predictions\keys\` on the laptop (`admin-keypair.json`, `buffer-keypair.json`, the program keypairs). Those are for program upgrades. The running site does not read them.

The API finds `.env` and the three keys only when systemd starts it with working directory `/opt/sol/backend`.

These already match the code, so they do not go in `.env`:

- Admin and treasury: `4tyf8awFN919MwuhDs7AkPN4gY6RJNawSwqisK3dcHdy`
- Result cosigner: `4gGayWwFTHeuwK2o1chizBDPaMwqUBUvmBdpaYXzFEHH`
- Program: `AaXABVJE79W6za8ntvto8wvtP6Kqgs5kQgHCS2uoHoaX`

## How the home network is opened

Friends use a hostname that points at the home's public address, for example `sol.example.com`. The router forwards **443** to this server's LAN address, and **80** as well when certificate renewal needs it.

Do not forward 22, 8080, 27017, or any other port. Do not forward anything to the laptop. SSH can stay unused. If the server has an SSH service for your own use, it stays reachable only from inside the house.

The API listens on every interface on port 8080. Nginx calls it at `127.0.0.1:8080`. Close 8080 to the rest of the house with the firewall in step 3, or a phone on the Wi-Fi can open the API directly and skip nginx.

This server sits on the same LAN as the other devices in the house. The website does not give friends a route to those devices. If someone took over this server, the server could start connections to them, because the router has no separate network for the server.

## 1. Build on the laptop

From `C:\solwalletapp`:

```powershell
.\build.ps1
```

That writes a Linux `sol-backend` in the repo root and the pages in `frontend\dist`. `VITE_API_URL` stays unset, so the pages call `/api` on the same host.

## 2. Fill the USB stick

Plug the stick in. Replace `E:` with its drive letter.

```powershell
$usb = "E:\sol"
New-Item -ItemType Directory -Force -Path "$usb\backend","$usb\keys","$usb\dist" | Out-Null
Copy-Item C:\solwalletapp\sol-backend "$usb\backend\sol-backend" -Force
Copy-Item C:\solwalletapp\.env "$usb\env" -Force
Copy-Item C:\solwalletapp\docker-compose.yml "$usb\docker-compose.yml" -Force
Copy-Item C:\solwalletapp\programs\predictions\keys\deployer-keypair.json "$usb\keys\" -Force
Copy-Item C:\solwalletapp\programs\predictions\keys\council-signer.json "$usb\keys\" -Force
Copy-Item C:\solwalletapp\programs\predictions\keys\reveal-keeper.json "$usb\keys\" -Force
Copy-Item C:\solwalletapp\frontend\dist\* "$usb\dist\" -Recurse -Force
```

Eject the stick from the laptop. It now holds the signing keys and `env`. Keep it with you until those files are on the server, then delete `E:\sol` from the stick. Linux hides names that start with a dot, which is why the stick carries `env` and the server renames it to `.env`.

## 3. Install at the server's keyboard

Plug the stick into the server. Find its mount with `lsblk` (often `/media/<you>/<stick-name>`). Below, that path is `/media/usb`. Then:

```bash
sudo mkdir -p /opt/sol/backend /opt/sol/programs/predictions/keys /var/www/sol
sudo cp /media/usb/sol/backend/sol-backend /opt/sol/backend/sol-backend
sudo cp /media/usb/sol/env /opt/sol/.env
sudo cp /media/usb/sol/docker-compose.yml /opt/sol/docker-compose.yml
sudo cp /media/usb/sol/keys/*.json /opt/sol/programs/predictions/keys/
sudo cp -a /media/usb/sol/dist/. /var/www/sol/
sudo chmod 755 /opt/sol/backend/sol-backend
sudo chmod 600 /opt/sol/.env /opt/sol/programs/predictions/keys/*.json
sudo chown -R www-data:www-data /var/www/sol
```

Unplug the stick when the copy finishes. Nginx must use `/var/www/sol` as the site root. Pointing it at `/opt/sol` would let visitors download the keys and `.env`.

MongoDB runs in Docker. On Ubuntu or Debian, install Docker first. `docker compose` is missing until this finishes:

```bash
sudo apt update
sudo apt install -y docker.io docker-compose-v2
sudo systemctl enable --now docker
docker --version
docker compose version
```

Then start MongoDB from `/opt/sol`:

```bash
cd /opt/sol
sudo docker compose up -d
sudo docker ps
```

`docker ps` should show `sol-mongodb`. The compose file publishes it only on `127.0.0.1:27017`. The API uses that address and the database name `sol`. The database starts empty. Copying the laptop events is optional and is at the bottom of this file.

Close port 8080 to every other device. Loopback stays open, which is how nginx reaches the API:

```bash
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw deny 8080/tcp
sudo ufw deny 27017/tcp
sudo ufw enable
```

`ufw` already accepts traffic on the loopback interface, so `127.0.0.1:8080` still answers.

## 4. systemd

Create `/etc/systemd/system/sol.service` in an editor on the server:

```ini
[Unit]
Description=Sol devnet API
After=network.target docker.service

[Service]
WorkingDirectory=/opt/sol/backend
ExecStart=/opt/sol/backend/sol-backend
Restart=always
StandardOutput=journal

[Install]
WantedBy=multi-user.target
```

The process reads `/opt/sol/.env` because the working directory is `/opt/sol/backend`.

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now sol
sudo systemctl status sol
curl -s http://127.0.0.1:8080/health
```

The log should name the reveal keeper and say automatic resolve is on. Health should return `{"status":"healthy"}`.

## 5. Nginx and the router

Create `/etc/nginx/sites-available/sol` on the server. This block is the default site, so an address or a hostname both reach the app. The stock nginx page stays up while `/etc/nginx/sites-enabled/default` is still enabled. Leave `listen 443 ssl` out until certbot adds the certificate lines. A `443 ssl` block with no certificate makes `nginx -t` fail, and nginx then keeps serving the old page.

```nginx
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;

    root /var/www/sol;
    index index.html;

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 150s;
    }

    location = /health {
        proxy_pass http://127.0.0.1:8080;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

```bash
sudo rm -f /etc/nginx/sites-enabled/default
sudo ln -sf /etc/nginx/sites-available/sol /etc/nginx/sites-enabled/sol
sudo nginx -t && sudo systemctl reload nginx
```

`try_files` is what makes `/events/...` and `/coupons` survive a refresh. The site is plain HTTP until the certificate below is in place. Phantom only signs in on HTTPS.

On the router, forward TCP 443 and TCP 80 to this server's LAN address. Forward nothing else for this app. Point a hostname, such as `sol.example.com`, at the home's public address. Certbot cannot issue a certificate for a bare IP.

Install certbot and let it edit the Sol site. Replace the name with the hostname that reaches this server:

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d sol.example.com
```

Certbot asks for an email and agreement to the terms, then writes the certificate into `/etc/letsencrypt/live/sol.example.com/` and adds a port 443 server to `/etc/nginx/sites-available/sol`. It reloads nginx when that succeeds. Open `https://sol.example.com` afterwards.

Renewal is already scheduled by the certbot package. Check it with `sudo certbot renew --dry-run`. When the home address changes, update the DNS name. The certificate stays valid as long as that name still reaches this server on port 80.

## 6. What friends do

They open `https://sol.example.com` and set Phantom to Devnet. Payments are devnet SOL to the treasury wallet above. You still pay winnings from that wallet. The server has no treasury key.

Sign-in is limited to 10 attempts per minute for the whole site, because every request arrives from nginx on `127.0.0.1`. A few people signing in one after another is fine.

## Later updates

On the laptop, run `.\build.ps1` again and copy the new `sol-backend` and `frontend\dist` onto the stick the same way as step 2. Leave `env` and the keys off the stick unless they changed.

At the server:

```bash
sudo systemctl stop sol
sudo cp /opt/sol/backend/sol-backend /opt/sol/backend/sol-backend.bak
sudo cp /media/usb/sol/backend/sol-backend /opt/sol/backend/sol-backend
sudo chmod 755 /opt/sol/backend/sol-backend
sudo rm -rf /var/www/sol/*
sudo cp -a /media/usb/sol/dist/. /var/www/sol/
sudo chown -R www-data:www-data /var/www/sol
sudo systemctl start sol
```

Unplug the stick. To roll the binary back: `sudo systemctl stop sol`, copy `sol-backend.bak` back to `sol-backend`, `sudo systemctl start sol`.

## Copy the current database (optional)

Do this only if the events already on the laptop should appear on the server. On the laptop, with `sol-mongodb` running:

```powershell
docker exec sol-mongodb mongodump --db sol --archive=/tmp/sol.archive
docker cp sol-mongodb:/tmp/sol.archive C:\solwalletapp\sol.archive
Copy-Item C:\solwalletapp\sol.archive E:\sol\sol.archive
```

At the server, after `sol-mongodb` is running:

```bash
sudo docker cp /media/usb/sol/sol.archive sol-mongodb:/tmp/sol.archive
sudo docker exec sol-mongodb mongorestore --archive=/tmp/sol.archive --drop
sudo docker exec sol-mongodb rm /tmp/sol.archive
sudo systemctl restart sol
```

Delete `sol.archive` from the laptop and from the stick after the restore.
