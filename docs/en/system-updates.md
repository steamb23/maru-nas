# System updates

"System Updates" in "All apps" shows updates for OMV and Debian packages, checks for updates, and installs them all. It does the same job as "Update Management › Updates" in the OMV Workbench, and only administrators can use it.

## Update list

- The list shows the update list that OMV keeps. OMV rebuilds it once a day and every time updates are checked or installed. "Last checked" is when OMV last rebuilt the list.
- "Check for updates" downloads the package lists from the repositories again (`apt-get update`). It usually takes 10 seconds to a minute.
- Changelogs for each package and update settings (pre-release updates, automatic updates and so on) are in the OMV Workbench. The links in the window only appear when the [OMV Workbench address](configuration.md#opening-the-omv-workbench) is set.
- When updates are available, administrators see an "Updates available" notice in the notification center.

## Installing all updates

- "Install all" installs every available update (`apt-get dist-upgrade`, the same as "Install updates" in the OMV Workbench). You can't pick individual packages.
- Services restart during the installation, so SMB/CIFS and NFS shares, Docker containers, and the OMV Workbench may drop for a moment. Install when the NAS is lightly used.
- If another package task (automatic security updates and so on) is using apt or dpkg, the installation doesn't start. Try again in a moment.
- If OMV has unapplied apt configuration changes, the update check doesn't start. Apply the pending changes in the OMV Workbench first. Updating the openmediavault package can leave configuration changes pending, so the first update check after "Install all" may be blocked for this reason.
- From OMV 8.5.10, OMV stops the installation if it would remove the openmediavault package. In that case, check the repository settings in the OMV Workbench.
- Only one check or installation runs at a time. If a task started by another administrator is running, you can start yours after it finishes.

## Progress and results

- Checks and installations run on the server. They keep running if you close the window or sign out, and the dashboard watches the task to the end and tells the administrator who started it how it went.
- The "Task" tab shows the output of the running task (what apt prints) as it arrives. The tab shows the most recent task, and you can reopen a finished task with "View output" on its result notification.
- While a task is running and the window is visible (not minimized, and the browser tab isn't hidden), your session doesn't expire.
- When the window is closed, the running task appears under "Background tasks" at the top of the notification center.
- Result notifications (succeeded, failed, result unknown) are only shown to the administrator who started the task, and you can dismiss them once you've read them. "Clear all" in the notification center dismisses every result notification at once and keeps notices about the current state, such as "Updates available". A task that finished while you were signed out shows up as a notification the next time you sign in. Each user keeps the 10 most recent results for 30 days.
- When a package that needs a restart (such as a kernel) is installed, a "Restart required" notice appears. "Restart…" on the notice or in the window opens the power dialog.

## Limitations

- While no administrator is signed in, the dashboard can't receive the output ("Waiting"). The task keeps running on the server, and the output continues when an administrator signs in again. If the NAS restarts in the meantime, the result is "unknown". Check the actual state with the update list and the "Restart required" notice.
- If you open the same task in the OMV Workbench's notification panel and watch it to the end, the dashboard can't read the result, so the result may be "unknown" or part of the output may be missing. The same happens if you stop the task in the OMV Workbench.
- When a task fails, the last second or so of output may be missing, because OMV doesn't hand out the remaining output of a failed task.
- Very long output keeps only the last 2–4 MiB of each task and drops the beginning.
- If OMV's session ends while the openmediavault package is being upgraded, you may be signed out of the dashboard too. Sign in again to keep watching the task.
