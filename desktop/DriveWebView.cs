using System.Windows.Input;
using Microsoft.Web.WebView2.Wpf;

namespace Teldrive.Desktop;

public sealed class DriveWebView : WebView2CompositionControl
{
    protected override void OnMouseDoubleClick(MouseButtonEventArgs e)
    {
        // The pinned SDK forwards a second button-down here, in addition to
        // OnMouseDown. Chromium already recognizes double clicks from the normal
        // down/up sequence; the extra event breaks folder actions and selection.
        // https://github.com/MicrosoftEdge/WebView2Feedback/issues/5099
    }
}
