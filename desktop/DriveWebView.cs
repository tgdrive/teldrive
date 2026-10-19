using Microsoft.Web.WebView2.Wpf;

namespace Teldrive.Desktop;

// A windowed WebView2 does not allocate the WPF composition control's D3D9
// capture surface, which can fail while restoring or starting in the tray.
// Views explicitly collapse this control before showing the WPF panels.
// Native Windows input also avoids composition's duplicated double-click event.
public sealed class DriveWebView : WebView2 { }
