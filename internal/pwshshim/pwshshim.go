// Package pwshshim lets the imux binary stand in for PowerShell 7 (pwsh) on
// Windows machines that only have Windows PowerShell: psmux starts popups
// with pwsh by name. imux puts a copy of itself named pwsh.exe in a folder of
// its own and adds that folder to psmux's PATH only when no real pwsh is
// installed; run as pwsh.exe, imux runs powershell.exe with the same
// arguments.
package pwshshim
