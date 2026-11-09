# System updates

"System Updates" in "All apps" shows updates for OMV and Debian packages, checks for updates, and installs them all. It does the same job as "Update Management › Updates" in the OMV Workbench, and only administrators can use it.

## Update list

- The list shows the update list that OMV keeps. OMV rebuilds it once a day and every time updates are checked or installed. "Last checked" is when OMV last rebuilt the list.
- "Check for updates" downloads the package lists from the repositories again (`apt-get update`). It usually takes 10 seconds to a minute.
- Update settings (pre-release updates, automatic updates and so on) are in the OMV Workbench. The link in the window only appears when the [OMV Workbench address](configuration.md#opening-the-omv-workbench) is set.
- When updates are available, administrators see an "Updates available" notice in the notification center.

## Package description and changelog

- The info button at the end of each row ("Description and changelog") shows that package's description and changelog inside the window.
- The description is shown as the original English text that comes with the package. OMV doesn't download translations of package descriptions.
- For packages from the Debian repositories, the update list only has a one-line summary. In that case the description of the currently installed version is shown instead, with a note saying so. Packages that are newly installed only show the summary.
- The changelog first shows the entries that are newer than the installed version. Select "Installed version and earlier" to expand the older entries.
- The first time you view a changelog, OMV downloads that package file. Large packages such as a kernel can take a few minutes. The downloaded file is reused when the update is installed later. A changelog that has been loaded once shows up right away when you close and reopen the dialog.
- While a check or installation is running, the changelog isn't loaded. It loads when the task finishes.
- When the [OMV Workbench address](configuration.md#opening-the-omv-workbench) is set, "View in OMV Workbench" opens the same package's changelog in the OMV Workbench.

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
- When a package that needs a restart (such as a kernel) is installed, a "Restart required" notice appears. "Restart…" on the notice or in the window opens the power dialog to restart right away, and "Schedule…" in the window sets a time for the restart ([Scheduling a restart](#scheduling-a-restart)).

## Scheduling a restart

Instead of restarting now, you can schedule the restart for a set time. To restart in the early morning when the NAS is lightly used, schedule it after installing the updates.

- Select "Schedule…" on the "Restart to finish applying updates" notice in the window, or choose "At a set time" under "Power" → "Restart" in the profile menu on the taskbar. Only administrators can schedule and cancel.
- Pick a time from 1 minute to 7 days ahead. The date and time are picked and shown in this browser's time zone. If the NAS is in a different time zone, the time on the NAS clock is shown as well.
- Scheduling and cancelling are root operations of the [privileged helper](helper.md), so you need to enter your password again to elevate to root. Accounts that can't elevate and installs without the helper can only use "Now".
- The schedule is kept by systemd-logind on the NAS. It stays if you close the browser, sign out, or the dashboard restarts.
- There can only be one schedule. Scheduling again replaces the previous one, and a shutdown scheduled outside the dashboard also becomes a restart.
- While a schedule exists, every user sees a "Restart scheduled" notice in the notification center. Schedules made outside the dashboard (such as `shutdown -r 03:00` over SSH) and scheduled shutdowns show up too, and administrators can cancel them from the dashboard. For a schedule made outside the dashboard, there is no way to tell who made it.
- After scheduling, the notice in the window changes to the scheduled time and the time left, with "Change schedule…" and "Cancel schedule".
- 10 minutes before the time, every open tab shows a warning notification, and from 1 minute before, a countdown appears at the top of the screen. Administrators can cancel the schedule from the countdown, or restart right away if it's a restart. At the time, the screen changes to "Restarting the NAS". Scheduled shutdowns show the same warnings.
- From 5 minutes before the time, systemd-logind blocks new logins by users other than root (`/run/nologin`), so new SSH and [web terminal](terminal.md) connections fail. Signing in to the dashboard isn't blocked.
- If the schedule is changed or cancelled in another tab or over SSH, open tabs pick it up within 1 minute 30 seconds at most. The countdown checks the schedule again 1 minute and 5 seconds before the time. A hidden browser tab can't check again, so it may switch to the restart screen at the time of a cancelled schedule. If the NAS hasn't gone down after 2 minutes, "Back to the dashboard" appears.

## Limitations

- While no administrator is signed in, the dashboard can't receive the output ("Waiting"). The task keeps running on the server, and the output continues when an administrator signs in again. If the NAS restarts in the meantime, the result is "unknown". Check the actual state with the update list and the "Restart required" notice.
- If you open the same task in the OMV Workbench's notification panel and watch it to the end, the dashboard can't read the result, so the result may be "unknown" or part of the output may be missing. The same happens if you stop the task in the OMV Workbench.
- When a task fails, the last second or so of output may be missing, because OMV doesn't hand out the remaining output of a failed task.
- Very long output keeps only the last 2–4 MiB of each task and drops the beginning.
- A very long changelog only shows its beginning (the newest entries). The earlier entries show up to 64 KiB from the installed version. See the OMV Workbench for the whole changelog.
- If the package has no changelog or OMV can't download the package file, the changelog can't be shown.
- If OMV's session ends while the openmediavault package is being upgraded, you may be signed out of the dashboard too. Sign in again to keep watching the task.
