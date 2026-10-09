param([int]$TargetPid)
Add-Type -TypeDefinition @'
using System;
using System.Text;
using System.Runtime.InteropServices;
using System.Collections.Generic;
public class DxPid {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumCb cb, int l);
  [DllImport("user32.dll")] public static extern int GetWindowThreadProcessId(IntPtr h, out int pid);
  [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetClassNameW(IntPtr h, StringBuilder s, int max);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern int GetWindowLongW(IntPtr h, int i);
  public delegate bool EnumCb(IntPtr h, int l);
  [StructLayout(LayoutKind.Sequential)] public struct R { public int L, T, Rt, B; }
}
'@
[DxPid]::SetProcessDPIAware() | Out-Null
$script:found = @()
$cb = [DxPid+EnumCb]{
  param($h, $l)
  $wpid = 0
  [DxPid]::GetWindowThreadProcessId($h, [ref]$wpid) | Out-Null
  if ($wpid -eq $TargetPid) {
    $cls = New-Object System.Text.StringBuilder 128
    [DxPid]::GetClassNameW($h, $cls, 128) | Out-Null
    $r = New-Object DxPid+R
    [DxPid]::GetWindowRect($h, [ref]$r) | Out-Null
    $script:found += ('0x{0:X} {1} rect={2},{3} {4}x{5} vis={6} ex=0x{7:X}' -f $h.ToInt64(), $cls.ToString(), $r.L, $r.T, ($r.Rt-$r.L), ($r.B-$r.T), [DxPid]::IsWindowVisible($h), [DxPid]::GetWindowLongW($h, -20))
  }
  return $true
}
[DxPid]::EnumWindows($cb, 0) | Out-Null
$script:found
'--- taskbar buttons bernama pet/overlay:'
Add-Type -AssemblyName UIAutomationClient
$root = [System.Windows.Automation.AutomationElement]::RootElement
$tb = $null
$tbs = $root.FindAll([System.Windows.Automation.TreeScope]::Children,
  (New-Object System.Windows.Automation.PropertyCondition(
    [System.Windows.Automation.AutomationElement]::ClassNameProperty, 'Shell_TrayWnd')))
foreach ($t in $tbs) { $tb = $t }
if ($tb) {
  foreach ($b in $tb.FindAll([System.Windows.Automation.TreeScope]::Descendants,
    [System.Windows.Automation.Condition]::TrueCondition)) {
    try {
      $n = $b.Current.Name
      if ($n -like '*pet*' -or $n -like '*overlay*' -or $n -like '*wails*') {
        'BUTTON: name={0} id={1}' -f $n, $b.Current.AutomationId
      }
    } catch { }
  }
}
'(selesai)'
