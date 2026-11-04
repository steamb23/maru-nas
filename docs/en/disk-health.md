# Disk health

"Disk Health" in "All apps" shows the SMART status and temperature of each disk, and when you select a disk it shows that disk's SMART details in tables. Only administrators can use it. SMART settings and self-test schedules are in the S.M.A.R.T. pages of the OMV Workbench.

## Disk list

- The list on the left shows each disk's status (Healthy, Warning, Failing, Unknown) and temperature. The status is OMV's assessment of the SMART values, and disks with problems come first.
- The list is read from OMV again only once a day (the first time it is opened after the SMART check time) and when you press "Check now". Every check reads all disks, so sleeping disks wake up. "Check now" can be pressed again 1 minute after the last check.
- Change the check time with `-smart-refresh-at` in [Configuration](configuration.md) or on the "Server" page of the Settings app.

## Details

- Selecting a disk in the list reads that disk's SMART data. A sleeping disk wakes up at this point, so reading can take from a few seconds to tens of seconds. That is why no disk is selected when the window opens.
- A disk that has been read is not read again when you select it again while the window is open. Press "Read again" when you need new values.
- In a narrow window the list and the details are shown one at a time. Press "Disks" in the details to go back to the list.
- The details are split into tabs.
    - Overview: a status notice, a summary such as temperature, power-on time and power cycles, and device information (model, serial number, firmware, capacity and so on). The summary shows reallocated, pending and uncorrectable sectors for SATA disks, and endurance used, available spare and media errors for NVMe disks.
    - Attributes (SATA): the SMART attribute table. A pre-failure item means the disk may fail soon once its current value drops to the threshold or below. "Show only items needing attention" shows only the items with problems.
    - Health (NVMe): the NVMe SMART/Health Information log. Critical warning, available spare, endurance used and media errors have a dot that shows their status.
    - Self-tests: recent self-test logs. Self-tests are scheduled and run in the OMV Workbench.
    - Raw output: the `smartctl --xall` output that OMV ran, as is. For SATA disks, the disk is read once more the first time this tab is opened.
- The serial number appears in the device information and in the raw output. Other identifiers such as the WWN are not in the device information, but they stay in the raw output as smartctl printed them. Remove such values before sending the raw output to someone else.
- If the [OMV Workbench address](configuration.md#opening-the-omv-workbench) is set, "View in OMV" opens the S.M.A.R.T. details page of the same disk, and "Schedule in OMV" on the Self-tests tab opens the self-test schedule page.

## Limitations

- Devices that don't pass SMART data through, such as disks behind a USB adapter, can't show details.
- If another window or another administrator is reading the same disk, reading continues after that read finishes.
- Many items in the details show values as smartctl printed them. Only common attribute names are translated, and the rest keep smartctl's names. The meaning of raw values can differ by manufacturer.
- OMV versions before 8.6 decide the list status of NVMe disks only from smartctl's self-assessment. Even if endurance used or available spare is past its limit, the list may show Healthy, so check the Health tab.
- If the raw output is very long, only the first 256 KiB is shown.
- If the NVMe self-test log can't be read as a table because the smartctl output format differs, you are told to check the Raw output tab.
