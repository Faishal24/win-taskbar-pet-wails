Add-Type -AssemblyName System.Windows.Forms
foreach ($s in [System.Windows.Forms.Screen]::AllScreens) {
  'primary={0} bounds={1} working={2}' -f $s.Primary, $s.Bounds, $s.WorkingArea
}
