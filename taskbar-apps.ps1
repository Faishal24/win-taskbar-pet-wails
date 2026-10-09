Add-Type -AssemblyName UIAutomationClient
$root = [System.Windows.Automation.AutomationElement]::RootElement
$tbcond = New-Object System.Windows.Automation.PropertyCondition(
  [System.Windows.Automation.AutomationElement]::ClassNameProperty, 'Shell_TrayWnd')
foreach ($tb in $root.FindAll([System.Windows.Automation.TreeScope]::Children, $tbcond)) {
  foreach ($b in $tb.FindAll([System.Windows.Automation.TreeScope]::Descendants,
    [System.Windows.Automation.Condition]::TrueCondition)) {
    try {
      if ($b.Current.AutomationId -like 'Appid:*') {
        $p = Get-Process -Id $b.Current.ProcessId -ErrorAction SilentlyContinue
        'appid={0} name={1} pid={2}({3})' -f $b.Current.AutomationId, $b.Current.Name,
          $b.Current.ProcessId, $p.ProcessName
      }
    } catch { }
  }
}
