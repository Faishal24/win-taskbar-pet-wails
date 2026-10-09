param([string]$Hwnd)
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public class DxVis {
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool IsWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool IsWindowEnabled(IntPtr h);
  [DllImport("user32.dll")] public static extern int GetWindowLongW(IntPtr h, int i);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out R r);
  [StructLayout(LayoutKind.Sequential)] public struct R { public int L, T, Rt, B; }
}
'@
[DxVis]::SetProcessDPIAware() | Out-Null
$h = [IntPtr]([Convert]::ToInt64($Hwnd, 16))
$r = New-Object DxVis+R; [DxVis]::GetWindowRect($h, [ref]$r) | Out-Null
"hwnd={0} alive={1} visible={2} enabled={3} rect={4},{5} {6}x{7} style=0x{8:X8} ex=0x{9:X8}" -f $Hwnd, [DxVis]::IsWindow($h), [DxVis]::IsWindowVisible($h), [DxVis]::IsWindowEnabled($h), $r.L, $r.T, ($r.Rt-$r.L), ($r.B-$r.T), [DxVis]::GetWindowLongW($h, -16), [DxVis]::GetWindowLongW($h, -20)
