# Windows PowerShell Script

# Get Users
$Users = Get-WmiObject -Class Win32_ComputerSystem | Select-Object -ExpandProperty UserName

# Get Device Info
$Hostname = $env:COMPUTERNAME
$Manufacturer = (Get-WmiObject Win32_ComputerSystem).Manufacturer
$Model = (Get-WmiObject Win32_ComputerSystem).Model

# Get OS Version
$OSVersion = (Get-WmiObject Win32_OperatingSystem).Caption

# Get Installed Software
$InstalledSoftware = Get-WmiObject -Class Win32_Product | Select-Object Name, Version

# Check for Pending Updates
$PendingUpdates = (New-Object -ComObject Microsoft.Update.Session).CreateUpdateSearcher().Search("IsInstalled=0").Updates | Select-Object Title

# Create the report
$Report = @"
Users: $Users
Device Info:
  Hostname: $Hostname
  Manufacturer: $Manufacturer
  Model: $Model
Current OS Version: $OSVersion
Installed Software: $(($InstalledSoftware | Out-String))
Pending OS/App Updates: $(($PendingUpdates | Out-String))
"@

# Write to file
$ReportPath = "C:\Temp\system_report.txt"
$Report | Out-File -FilePath $ReportPath

# Prompt for network credentials
$NetCredentials = Get-Credential

# Mount the network drive and copy the report
$NetworkPath = "\\netid.washington.edu\wfs\cas-deanery\IT\SPHSC-Servers"
New-PSDrive -Name "Z" -PSProvider FileSystem -Root $NetworkPath -Credential $NetCredentials

# Copy the report
Copy-Item $ReportPath -Destination "Z:\system_report.txt"

# Remove the mapped drive
Remove-PSDrive -Name "Z"

Write-Host "System report saved to the network drive."
