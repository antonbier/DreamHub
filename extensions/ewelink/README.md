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
- `key` — the devicekey. It's only in the owner's eWeLink account, never on the network: it has to be fetched once from eWeLink's API with a developer account (dev.ewelink.cc). A helper for that one-time fetch comes once a developer account exists to build it against.
- `offWarning` — added to the preview before switching **off**. Use it for anything that mustn't lose power unannounced.
- `lang` — `de` or `en` for the card's texts (an extension owns no DreamHub locale file).

After editing: `systemctl restart dreamhub-ext-ewelink`. The file holds secrets: owned by the extension's user, mode 600.

## Network

Unlike the template's default, this extension needs the LAN (mDNS multicast, each device's port 8081), so its unit allows private address ranges and `224.0.0.251` besides loopback. On systemd 232 (the reference box) that allowance isn't enforced at all — see `packaging/extension-template/README.md`.

## Confirmed on the reference box (2026-10-12)

The owner's Sonoff S60TPF answers mDNS as `eWeLink_1002a17548` on `192.168.1.64:8081`, `type=plug`, `encrypt=true` (the raw answer is `testdata/s60tpf-mdns.b64`, used by the tests). Registered with DreamHub, the card shows it with "device key missing". Reading and switching wait for the devicekey.

## Building

```sh
ARCH=arm64 sh extensions/ewelink/packaging/build-deb.sh   # → dist/dreamhub-ext-ewelink_0.1.0_arm64.deb
cd extensions/ewelink && go test ./...
```
