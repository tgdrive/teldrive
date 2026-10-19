using System.Runtime.InteropServices;
using System.Windows;
using System.Windows.Interop;

namespace Teldrive.Desktop;

internal sealed class WindowFullscreen(Window window)
{
    private Rect bounds;
    private WindowState state;
    private WindowStyle style;
    private ResizeMode resize;
    private bool topmost;
    public bool Active { get; private set; }
    public void Set(bool value)
    {
        if (Active == value) return;
        if (value) {
            state=window.WindowState;bounds=state==WindowState.Normal?new Rect(window.Left,window.Top,window.Width,window.Height):window.RestoreBounds;
            style=window.WindowStyle;resize=window.ResizeMode;topmost=window.Topmost;
            var handle=new WindowInteropHelper(window).Handle;var screen=System.Windows.Forms.Screen.FromHandle(handle).Bounds;
            window.WindowState=WindowState.Normal;window.WindowStyle=WindowStyle.None;window.ResizeMode=ResizeMode.NoResize;window.Topmost=true;
            _=SetWindowPos(handle,0,screen.X,screen.Y,screen.Width,screen.Height,0x0004|0x0010|0x0020);
        } else {
            window.WindowState=WindowState.Normal;window.WindowStyle=style;window.ResizeMode=resize;window.Topmost=topmost;
            window.Left=bounds.Left;window.Top=bounds.Top;window.Width=bounds.Width;window.Height=bounds.Height;window.WindowState=state;
        }
        Active=value;
    }
    [DllImport("user32.dll")] private static extern bool SetWindowPos(nint window,nint after,int x,int y,int width,int height,uint flags);
}
