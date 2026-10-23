# File explorer

"Files" in "All apps" and on the desktop is a window for browsing your home folder and the OMV shared folders. Signing in once with your OMV account shows every shared folder with your own file system permissions.

Files are handled by the [privileged helper](helper.md) with the signed-in user's permissions, so if the helper is not installed, the window explains why when it opens and cannot be used. Administrators can put a check in front of the file explorer with the [security hardening mode](security.md).

## Visible folders and permissions

- At first you see your home folder and the shared folders. A regular user's session cannot get the shared folder list from OMV, so the dashboard saves the list read when an administrator signs in and uses that. After adding or changing shared folders, an administrator signs in again or presses "Read now" under "File explorer" on the "Server" page of the Settings app. If the list has never been read, the disks under `/srv` are shown instead.
- What you can read and write follows the file system permissions (owner, mode, ACL). These are the same permissions as over SSH, and administrators also browse with their own account's permissions.
- The details panel that opens when you click a file shows the owner, the group, the mode, whether there is an ACL, and your access. Administrators also get a link to the ACL editor in OMV.
- Shared folders on the root file system (openmediavault-sharerootfs, for example `/kvm`) cannot be opened yet.

### Following OMV shared folder privileges

OMV shared folder privileges are settings for services such as SMB and FTP, so the file explorer does not follow them by default. When an administrator turns on "Follow OMV shared folder privileges" on the "Server" page of the Settings app (off by default), the file explorer applies them with the same rules as SMB. The first row from the top that matches applies.

| Privileges set in OMV | File explorer |
|---|---|
| "No access" for the user or any of their groups | The shared folder is hidden |
| At least one "Read/Write" | Writable |
| Only "Read only" | Read only (a lock shows in the sidebar) |
| No privileges set at all | Open to everyone |

- File system permissions still apply, and administrators also follow their own account's privileges.
- Turning it on reads the shared folder list again. If you change privileges in OMV afterwards, press "Read now". If you changed a user's groups, the change takes effect when that user signs in again.
- This filtering is done by the dashboard, so to block access at the privileged helper level, change the shared folder's ACL in OMV.

### Show disk tops

When an administrator turns on "Show disk tops" on the "Server" page of the Settings app (off by default), administrators can see beyond the shared folders, up to the top of each disk under `/srv`. Shared folders hidden by following shared folder privileges cannot be opened here either.

## Browsing and searching

- The button at the right of the toolbar switches between list view and icon view, and the sort button next to it sorts. The view you choose carries over to windows you open later.
- Folders with tens of thousands of items open right away. If a folder holds more than 300,000 items, only some of them are shown. Sorting by date modified or size changes after the details of every item have been read, and while they are read, a progress bar and a cancel button appear.
- The search box filters names in the current folder. Turn on "Search subfolders" next to it and press Enter to list, with their locations, the items in the current folder and every folder below it whose names contain the search term.
    - The first 2000 are shown, and folders you cannot read are skipped. Results appear as they are found, and "Stop" stops the search.
    - From the results you can open folders, preview or download files, and go to the folder that contains an item with "Open location".
- You can open several windows to see two folders side by side. When the contents of a folder change in one window, other windows showing the same folder update right away.

## Preview

Images, video and audio, PDFs and text files open in the "Preview" window when you double-click them, press Enter, choose "Preview" in the right-click menu, or press the preview button in the details panel.

- Only formats the browser can open are shown. Download HEIC, MKV and the like to open them. Only the first 1 MiB of a text file is shown.
- There is one Preview window, and opening another file switches it to that file. While you watch a video, the session is extended even without input.

## Downloading and uploading

- **Download**: with "Download" in the details panel or in the file's right-click menu. The browser's download manager does the download, so an interrupted download can be resumed.
- **Upload**: "Upload" lets you choose several files to upload to the current folder. You can also drag files from your computer into the window. Dropping them on a folder row uploads into that folder, and dropping them on an empty area uploads into the current folder. Folders cannot be uploaded yet, and folders dragged along are skipped and reported.
- If a file with the same name exists, it asks whether to overwrite it. Overwriting replaces it with a new file, so the owner becomes the uploading user (permission bits are kept where possible). If you can write to the folder, you can overwrite files even without write permission on them, just as on Linux write permission on a folder lets you delete and recreate the files in it.
- When an upload takes a while, the "File tasks" window shows the amount uploaded, the speed and the time left. While uploading, the session is extended even without input, and the upload continues even if you close the explorer window. Closing the "File tasks" window or leaving the page cancels it, and files that failed or were canceled can be uploaded again from that window. When an upload or copy fails, or a task that took long enough to open the "File tasks" window finishes, you are also notified in the bottom-right corner and in the taskbar's notification center. These notifications are kept only in that browser tab and disappear when you reload the page.
- A transfer is cut off after 60 seconds without progress, and can last up to 6 hours at a time. Downloads and uploads that have already started run to the end even if you sign out or the security policy is raised.

## Organizing and the trash

- Create, rename and delete folders with "New folder" on the toolbar and "Rename" and "Delete" in the right-click menu. The details panel also has rename and delete buttons. Right-clicking an empty area offers New folder, Upload and Select all.
- Folders shown in the sidebar (home folder, shared folders, disks) and the folders containing them cannot be renamed, moved or deleted. Manage shared folders in OMV.
- Deleting or moving several items handles them one at a time, and if it takes a while, a progress bar and a "Stop" button appear. Stopping finishes the current item and then stops. Deleting a single folder stops after 10 minutes, and deleting it again continues with the remaining items.

### Trash

Deleting moves items to the trash. It is the same trash, in the same place and format, as trash-cli on the server, so `trash-list`, `trash-restore` and `trash-empty` on the server work on the same trash. It is separate from the SMB recycle bin (`.recycle`).

- Items from your home folder go to the home trash (`~/.local/share/Trash`), and items on a disk go to `.Trash/<uid>` or `.Trash-<uid>` at the top of that disk. Nothing is copied to another disk.
- The top of an OMV data disk is usually owned by root, so users cannot create a trash folder there. On such disks, it warns before deleting that the deletion is permanent. When an administrator presses "Create trash" for a disk under "File explorer" on the "Server" page of the Settings app (root elevation required), a `.Trash` folder shared by all users is created on that disk, and from then on deleted items go to the trash.
- "Trash" at the end of the sidebar shows the items with their original location and date deleted, and lets you restore them, delete them permanently or empty the trash. If the original location has an item with the same name, restoring asks for a new name. Dragging selected items onto "Trash" in the sidebar also moves them to the trash.
- Choosing "Delete permanently…" in the confirmation dialog, or pressing Shift+Delete, deletes without going through the trash. Permanent deletion cannot be undone, and for a folder it deletes all the files and folders inside.

## Moving and copying

Drag selected items onto a folder row, the path at the top, a folder in the sidebar or another explorer window to move them. You can drag with a mouse or a pen, but not with touch.

- In another explorer window, dropping on an empty area or a file row also moves the items into the folder that window shows.
- Dropping on a different disk copies instead of moving, and the drag label shows "Copy" while you drag. Your home folder and the data disks are different disks too. Items in a folder you can only read can still be copied to another disk.
- If the destination already has an item with the same name, that item is skipped and reported.
- When copying takes a while, the "File tasks" window shows the amount copied, the number of files, the speed and the time left. When everything is copied, the window closes by itself, and if some items could not be copied, it stays open with the reasons. Canceling the copy in the window, or closing the window, deletes the item being copied and stops, and items already copied stay. The copy continues even if you close the explorer window.
- Copying carries over only the contents and the date modified. Permissions, owner and ACL follow the destination folder's defaults as with uploads, symbolic links are recreated as links, and special files inside folders (FIFOs, sockets, devices) are skipped.

## Keyboard shortcuts

| Key | Action |
|---|---|
| Enter, double-click | Open a folder, preview a file |
| Backspace | Go to the parent folder |
| F2 | Rename |
| Delete | Move to the trash |
| Shift+Delete | Delete permanently |
| Click while holding Ctrl (⌘ on macOS) or Shift | Select several items |
| Ctrl+A (⌘+A on macOS) | Select all |
| Esc | Clear the selection |

## Logging

The privileged helper's audit log (`journalctl -u maru-helper`) records the results of downloads and previews, and write operations (uploads, new folders, renames and moves, copies, deletions, trash operations) with their paths. Folder listings, reading details, trash listings and subfolder searches are not recorded.
