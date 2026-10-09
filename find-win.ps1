Add-Type -AssemblyName UIAutomationClient
$root = [System.Windows.Automation.AutomationElement]::RootElement
$kids = $root.FindAll([System.Windows.Automation.TreeScope]::Children,
  [System.Windows.Automation.Condition]::TrueCondition)
foreach ($k in $kids) {
  try {
    if ($k.Current.Name -like '*taskbar-pet-overlay*') {
      'hwnd-title match: name={0} cls={1} pid={2}' -f $k.Current.Name, $k.Current.ClassName, $k.Current.ProcessId
    }
  } catch { }
}
