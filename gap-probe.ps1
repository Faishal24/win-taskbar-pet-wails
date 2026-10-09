$ErrorActionPreference = 'SilentlyContinue'
$ProgressPreference = 'SilentlyContinue'
Add-Type -AssemblyName UIAutomationClient
function Get-PetGap {
  try {
    $root = [System.Windows.Automation.AutomationElement]::RootElement
    $cond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::ClassNameProperty, 'Shell_TrayWnd')
    $tb = $root.FindFirst([System.Windows.Automation.TreeScope]::Children, $cond)
    if ($null -eq $tb) { return }
    $tbRect = $tb.Current.BoundingRectangle
    if ($tbRect.Width -le 0) { return }
    $all = $tb.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
    $startX = -1.0
    $groupRight = -1.0
    $trayX = 1e9
    $leftBarrier = -1.0
    foreach ($e in $all) {
      $r = $e.Current.BoundingRectangle
      if ($r.Width -le 0) { continue }
      $id = $e.Current.AutomationId
      if ($id -eq 'StartButton') { $startX = $r.X }
      elseif ($id -like 'Appid:*') { if ($r.Right -gt $groupRight) { $groupRight = $r.Right } }
      elseif (($id -eq 'SystemTrayIcon') -or ($id -eq 'NotifyItemIcon')) { if ($r.X -lt $trayX) { $trayX = $r.X } }
      elseif (($e.Current.ControlType -eq [System.Windows.Automation.ControlType]::Button) -and ($r.X -lt $tbRect.X + $tbRect.Width * 0.5)) { if ($r.Right -gt $leftBarrier) { $leftBarrier = $r.Right } }
    }
    $tbX = [int]$tbRect.X
    $tbY = [int]$tbRect.Y
    $tbW = [int]$tbRect.Width
    $tbH = [int]$tbRect.Height
    $leftStart = [Math]::Max($leftBarrier + 1, [double]$tbX)
    $leftGap = 0.0
    if ($startX -ge 0) { $leftGap = $startX - $leftStart }
    $rightStart = [double]($tbX + [Math]::Floor($tbW / 2))
    if ($groupRight -ge 0) { $rightStart = $groupRight + 4 }
    $rightGap = 0.0
    if ($trayX -lt 1e8) { $rightGap = $trayX - 4 - $rightStart }
    $side = 'none'
    $gx = 0
    $gw = 0
    if (($leftGap -ge $rightGap) -and ($leftGap -gt 40)) {
      $side = 'left'
      $gx = [int][Math]::Floor($leftStart)
      $gw = [int][Math]::Floor($leftGap)
    } elseif ($rightGap -gt 40) {
      $side = 'right'
      $gx = [int][Math]::Ceiling($rightStart)
      $gw = [int][Math]::Floor($rightGap)
    } else { return }
    $obj = [ordered]@{
      taskbar = @{ x = $tbX; y = $tbY; w = $tbW; h = $tbH }
      gap = @{ x = $gx; y = $tbY; w = $gw; h = $tbH }
      side = $side
    }
    ConvertTo-Json -InputObject $obj -Compress
  } catch { }
}
Get-PetGap
while ($true) { Start-Sleep -Seconds 5; Get-PetGap }
