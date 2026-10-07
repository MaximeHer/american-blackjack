# Releve du banc d'essai, pour le critere 1 de la grille.
#
# Les niveaux de cache rapportes par Win32_CacheMemory sont decales de 2 :
# Level=3 designe le L1, Level=4 le L2, Level=5 le L3. C'est une particularite
# de l'enumeration WMI, pas une erreur de lecture.

$ErrorActionPreference = 'Continue'

function Section($t) { Write-Output ""; Write-Output "--- $t ---" }

Write-Output "RELEVE DU BANC D'ESSAI"
Write-Output ("Date : " + (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'))

Section "Processeur"
$cpu = Get-CimInstance Win32_Processor
Write-Output ("Modele              : " + $cpu.Name.Trim())
Write-Output ("Coeurs physiques    : " + $cpu.NumberOfCores)
Write-Output ("Coeurs logiques     : " + $cpu.NumberOfLogicalProcessors)
Write-Output ("Frequence nominale  : " + $cpu.MaxClockSpeed + " MHz")
Write-Output ("Frequence courante  : " + $cpu.CurrentClockSpeed + " MHz")
Write-Output ("Charge au releve    : " + $cpu.LoadPercentage + " %")

Section "Hierarchie de cache"
$levels = @{ 3 = 'L1'; 4 = 'L2'; 5 = 'L3' }
Get-CimInstance Win32_CacheMemory | Sort-Object Level | ForEach-Object {
  $name = $levels[[int]$_.Level]
  if (-not $name) { $name = "niveau $($_.Level)" }
  Write-Output ("{0,-19} : {1} Ko total" -f $name, $_.InstalledSize)
}
Write-Output "Ligne de cache      : 64 octets"

Section "Memoire"
$dimms = @(Get-CimInstance Win32_PhysicalMemory)
$total = [math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory / 1GB, 1)
Write-Output ("Capacite totale     : " + $total + " Go utilisables")
Write-Output ("Modules installes   : " + $dimms.Count)
foreach ($d in $dimms) {
  Write-Output ("  " + [math]::Round($d.Capacity / 1GB, 0) + " Go @ " + $d.Speed + " MT/s  " + $d.Manufacturer.Trim() + " " + $d.PartNumber.Trim())
}
if ($dimms.Count -eq 1) {
  Write-Output "ATTENTION           : un seul module, donc fonctionnement en SIMPLE CANAL."
  Write-Output "                      La bande passante memoire est divisee par deux par"
  Write-Output "                      rapport a une configuration a deux modules."
}

Section "Systeme"
$os = Get-CimInstance Win32_OperatingSystem
Write-Output ("Systeme             : " + $os.Caption)
Write-Output ("Version             : " + $os.Version + " build " + $os.BuildNumber)

Section "Runtime Go"
Write-Output ("Version             : " + (go version))
Write-Output ("GOMAXPROCS          : " + (go env GOMAXPROCS))
Write-Output ("GOARCH / GOOS       : " + (go env GOARCH) + " / " + (go env GOOS))
Write-Output ("GOTOOLCHAIN         : " + (go env GOTOOLCHAIN))

Section "Conditions de mesure"
$bat = Get-CimInstance Win32_Battery -ErrorAction SilentlyContinue
if ($bat) {
  $onAC = ($bat.BatteryStatus -eq 2)
  Write-Output ("Alimentation        : " + $(if ($onAC) { "SECTEUR" } else { "BATTERIE -- mesure non valide" }))
  Write-Output ("Charge batterie     : " + $bat.EstimatedChargeRemaining + " %")
} else {
  Write-Output "Alimentation        : secteur (pas de batterie detectee)"
}

$load = $cpu.LoadPercentage
$verdict = if ($load -lt 5) { "OK, machine au repos" } else { "ATTENTION, charge $load % -- fermer les processus parasites" }
Write-Output ("Etat de la machine  : " + $verdict)

Write-Output ""
Write-Output "Rappel : le Ryzen 5600H est un processeur mobile. Une mesure prise sur"
Write-Output "batterie ou machine chargee peut etre fausse d'un facteur 2. Voir"
Write-Output "docs/02-protocole-de-mesure.md section 4.1."
