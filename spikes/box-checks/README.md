# Box checks

Read-only scripts that collect what can only be learned on the reference box, so a session with the box is short and every write afterwards is a deliberate, approved one. Each prints a report; nothing on the box changes except files in `/tmp`.

| Script | For | Collects |
|---|---|---|
| [`channelhub.sh`](channelhub.sh) | ChannelHub (#26) | Channel counts for the references the backend uses (all TV/radio, providers, bouquet roots), a provider's and a bouquet's listing from WebControl and from BouquetEditor, the BouquetEditor plugin's Python source as `/tmp/bouqueteditor-src.tgz` (positions, backup, restore read instead of tried), the reload endpoint, picon sets on disk and in the feed |
| [`../adscan/phase0.sh`](../adscan/phase0.sh) | AdScan (#44) | FFmpeg build, `.ts.ap`/`.ts.cuts` layout, cut-list and "seen" handling in Enigma2 and AdvancedMovieSelection, timings |

## A session with the box

1. Is anything recording or playing? `wget -q -O - http://127.0.0.1/web/timerlist | grep -c '<e2state>2</e2state>'` must be `0` before anything heavy (the AdScan timings) — the scripts run under `nice`, but a recording comes first.
2. Copy the scripts (`scp -O`, the box has no SFTP) and run them; fetch the reports and the tarball:
   ```sh
   scp -O spikes/box-checks/channelhub.sh spikes/adscan/phase0.sh root@<box>:/tmp/
   ssh root@<box> 'sh /tmp/channelhub.sh > /tmp/channelhub-check.txt 2>&1;
                   sh /tmp/phase0.sh /media/hdd/movie/<recording>.ts > /tmp/adscan-phase0.txt 2>&1'
   scp -O root@<box>:/tmp/channelhub-check.txt root@<box>:/tmp/adscan-phase0.txt root@<box>:/tmp/bouqueteditor-src.tgz .
   ```
3. For AdScan: copy one recording of about an hour with ad breaks (with its `.ap`, `.cuts`, `.meta`) to the development machine — reading only.
4. Confirmed facts go into `ARCHITECTURE.md`.
5. Only then, each one approved on its own: the first ChannelHub writes (one change, read back, restored), following `docs/CHANNELHUB_PLAN.md`.
