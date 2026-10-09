using System.ComponentModel;
using System.Diagnostics;
using System.Runtime.InteropServices;

namespace Teldrive.Desktop;

// Windows closes all owned service processes if the desktop window crashes.
internal sealed class WindowsJob : IDisposable
{
    private nint handle = CreateJobObjectW(0, null);
    public WindowsJob()
    {
        if (handle == 0) throw new Win32Exception(Marshal.GetLastWin32Error());
        var limits = new ExtendedLimits { Basic = new BasicLimits { Flags = 0x2000 } };
        var memory = Marshal.AllocHGlobal(Marshal.SizeOf<ExtendedLimits>());
        try { Marshal.StructureToPtr(limits, memory, false); if (!SetInformationJobObject(handle, 9, memory, (uint)Marshal.SizeOf<ExtendedLimits>())) throw new Win32Exception(Marshal.GetLastWin32Error()); }
        catch { Dispose(); throw; }
        finally { Marshal.FreeHGlobal(memory); }
    }
    public void Attach(Process process) { if (!AssignProcessToJobObject(handle, process.Handle)) { var error = Marshal.GetLastWin32Error(); if (!process.HasExited) process.Kill(true); throw new Win32Exception(error); } }
    public void Dispose() { if (handle != 0) { CloseHandle(handle); handle = 0; } }
    [StructLayout(LayoutKind.Sequential)] private struct BasicLimits { public long ProcessTime, JobTime; public uint Flags; public nuint MinimumWorkingSet, MaximumWorkingSet; public uint ActiveProcesses; public nuint Affinity; public uint PriorityClass, SchedulingClass; }
    [StructLayout(LayoutKind.Sequential)] private struct IoCounters { public ulong ReadOperations, WriteOperations, OtherOperations, ReadBytes, WriteBytes, OtherBytes; }
    [StructLayout(LayoutKind.Sequential)] private struct ExtendedLimits { public BasicLimits Basic; public IoCounters Io; public nuint ProcessMemory, JobMemory, PeakProcessMemory, PeakJobMemory; }
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)] private static extern nint CreateJobObjectW(nint attributes, string? name);
    [DllImport("kernel32.dll", SetLastError = true)] [return: MarshalAs(UnmanagedType.Bool)] private static extern bool SetInformationJobObject(nint job, int informationClass, nint information, uint length);
    [DllImport("kernel32.dll", SetLastError = true)] [return: MarshalAs(UnmanagedType.Bool)] private static extern bool AssignProcessToJobObject(nint job, nint process);
    [DllImport("kernel32.dll")] [return: MarshalAs(UnmanagedType.Bool)] private static extern bool CloseHandle(nint handle);
}
