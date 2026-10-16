# dreamhub-ext-ewelink

DreamHub's reference extension (`docs/EXTENSIONHUB_PLAN.md`, E4, #46): eWeLink/Sonoff devices with **LAN control** switched on, read and switched on the local network — no eWeLink cloud at run time. It shows up as one "eWeLink" card in AppsHub → Extensions: per device its state (on/off), readings if the device reports any (power, voltage, current), and a switch button that goes through DreamHub's preview-and-confirm like everything else.

It is also the worked example for anyone building an extension: it talks to DreamHub only through the public contract in [`docs/BUILDING_EXTENSIONS.md`](../../docs/BUILDING_EXTENSIONS.md) and imports nothing from DreamHub's own code — its own Go module, standard library only.

## How it works

- **Finding devices**: mDNS, `_ewelink._tcp.local`, every 10 s. Each device announces its id, type, a sequence number and its state in the TXT record.
- **Reading state**: with LAN control on, that state is encrypted — AES-128-CBC, the key being MD5 of the device's own *devicekey*, the IV announced alongside. Without the key a device is found but can't be read or switched; the card says so.
- **Switching**: `POST http://<device>:8081/zeroconf/switch` with the encrypted command; the extension then waits until the device announces the new state. Apply refuses (`409`) when the device changed since the preview — someone switched it from the app in between.

## Setting it up

Install the `.deb` (it depends on DreamHub ≥ 0.5.0). It runs as its own user `dreamhub-ext-ewelink` and keeps its configuration in `/var/lib/dreamhub-ext-ewelink/config.json`:

```json
{
  "lang": "de",
  "devices": [
    {
      "id": "1002a17548",
      "key": "<the devicekey from your eWeLink account>",
      "name": "Unraid-Steckdose",
      "offWarning": "Versorgt den Unraid-Server — vorher sauber herunterfahren."
    }
  ]
}
```

- `id` — the device id; a device that's found but not configured shows up on the card with it.
- `key` — the devicekey. It's only in the owner's eWeLink account, never on the network: it is fetched once from eWeLink's cloud through the owner's own developer application (dev.ewelink.cc → Platform → App Management, type OAuth 2.0 with a redirect address). In the card's **Settings**: enter the app ID, the app secret, the region and the redirect address, save; a **Log in to eWeLink** link appears — open it, log in, copy the whole address of the page you land on (`<redirect>?code=…&region=…`) into *Address after logging in* and save within 30 seconds. The extension trades the code for a token, reads the account's device list and stores the key of every device it finds on the network. The login is kept (token refreshed by itself) only to read the consumption history.
- `offWarning` — added to the preview before switching **off**. Use it for anything that mustn't lose power unannounced.
- `lang` — `de` or `en` for the card's texts (an extension owns no DreamHub locale file).

After editing: `systemctl restart dreamhub-ext-ewelink`. The file holds secrets: owned by the extension's user, mode 600.

## Consumption on the start page

The S60TPF announces `power`, `voltage`, `current`, `dayKwh` and `monthKwh` on the LAN itself (confirmed on the box 2026-10-16), so the card needs no cloud for them. For plugs that don't, and with a login stored, once a quarter of an hour the extension asks the cloud for the device's `hundredDaysKwh` history and live values (the S60TPF answers that request with error 4002, "control failed" — it is then not asked again). The card shows **Power**, **Today** and **This month**, and flags the on/off status and this list for DreamHub's start page (schema 3, `docs/BUILDING_EXTENSIONS.md`). Reads only — switching never goes through the cloud. That is about 6 calls per device per hour against the free application's 50 000 a month. Which of these a particular plug reports is decided by the plug: what it doesn't send isn't shown.

## Switching, and what is logged

The extension tries the LAN first (`/zeroconf/switches` for a plug that reports `switches`, `/zeroconf/switch` otherwise). **The S60TPF (firmware 1.2.0) answers every encrypted LAN command with error 400** — also with a wrong key, a different request order and `Connection: close`, so no variant tried on the box works (2026-10-16; reading its announcements works). When that happens and an eWeLink login is stored, the same switch goes through the cloud (`POST /v2/device/thing/status`, allowed for a standard-role application) and the extension waits until the plug announces the new state. Without a login the preview works but applying fails with the reason. The numbers the plug announces on the LAN are whole numbers in hundredths (voltage 23442 = 234.42 V).

Logs: the unit's own journal (`journalctl -u dreamhub-ext-ewelink`, also Device → Administration → Services → Logs): every switch (LAN failure, cloud fallback, result), cloud problems, registration. DreamHub's own log (`dreamhub`) names the extension's refusal text when an apply fails.

## Network

Unlike the template's default, this extension needs the LAN (mDNS multicast, each device's port 8081), so its unit allows private address ranges and `224.0.0.251` besides loopback. On systemd 232 (the reference box) that allowance isn't enforced at all — see `packaging/extension-template/README.md`.

## Confirmed on the reference box (2026-10-12)

The owner's Sonoff S60TPF answers mDNS as `eWeLink_1002a17548` on `192.168.1.64:8081`, `type=plug`, `encrypt=true` (the raw answer is `testdata/s60tpf-mdns.b64`, used by the tests). Registered with DreamHub, the card shows it with "device key missing". Reading and switching wait for the devicekey.

## Building

```sh
ARCH=arm64 sh extensions/ewelink/packaging/build-deb.sh   # → dist/dreamhub-ext-ewelink_0.1.0_arm64.deb
cd extensions/ewelink && go test ./...
```
