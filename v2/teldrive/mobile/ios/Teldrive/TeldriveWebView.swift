import SwiftUI
import WebKit

struct TeldriveWebView: UIViewRepresentable {
    @ObservedObject var store: BrowserStore
    func makeCoordinator() -> Coordinator { Coordinator(store: store) }
    func makeUIView(context: Context) -> WKWebView {
        store.webView.navigationDelegate = context.coordinator
        store.webView.uiDelegate = context.coordinator
        store.webView.configuration.userContentController.removeScriptMessageHandler(forName: "teldriveMobile")
        store.webView.configuration.userContentController.addScriptMessageHandler(context.coordinator, contentWorld: .page, name: "teldriveMobile")
        return store.webView
    }
    func updateUIView(_ view: WKWebView, context: Context) {}
    static func dismantleUIView(_ view: WKWebView, coordinator: Coordinator) { view.configuration.userContentController.removeScriptMessageHandler(forName: "teldriveMobile"); view.navigationDelegate = nil; view.uiDelegate = nil }

    @MainActor
    final class Coordinator: NSObject, WKNavigationDelegate, WKUIDelegate, WKDownloadDelegate, WKScriptMessageHandlerWithReply {
        let store: BrowserStore
        private var destinations: [ObjectIdentifier: URL] = [:]
        init(store: BrowserStore) { self.store = store }
        func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage, replyHandler: @escaping (Any?, String?) -> Void) {
            guard message.frameInfo.isMainFrame, let server = store.server, let source = message.frameInfo.request.url, ServerAddress.sameOrigin(server, source), let request = message.body as? [String: Any] else { replyHandler(nil, "Solicitud no válida."); return }
            if request["method"] as? String == "save" {
                do {
                    guard let contents = request["contents"] as? String, let data = contents.data(using: .utf8), data.count <= 2 * 1024 * 1024 else { throw URLError(.dataLengthExceedsMaximum) }
                    let folder = FileManager.default.temporaryDirectory.appendingPathComponent("TeldriveDownloads", isDirectory:true).appendingPathComponent(UUID().uuidString, isDirectory:true)
                    try FileManager.default.createDirectory(at:folder, withIntermediateDirectories:true)
                    let name = URL(fileURLWithPath:request["filename"] as? String ?? "archivo.txt").lastPathComponent
                    let url = folder.appendingPathComponent(name.isEmpty ? "archivo.txt" : name)
                    try data.write(to:url, options:.atomic); store.downloaded = DownloadedFile(url:url); replyHandler(true,nil)
                } catch { replyHandler(nil,"No se pudo guardar el archivo en el dispositivo.") }
                return
            }
            guard request["method"] as? String == "download", let raw = request["url"] as? String, let url = URL(string: raw), ServerAddress.sameOrigin(server, url),
                  url.path.range(of: #"^/api/v1/(playback|files/[^/]+/content/[^/]+|public/shares/[^/]+/(files/[^/]+/)?content/[^/]+)$"#, options: .regularExpression) != nil else { replyHandler(nil, "Solicitud de descarga no válida."); return }
            if store.downloading { replyHandler(nil, "Espera a que termine la descarga anterior."); return }
            store.downloading = true
            Task { @MainActor in
                do {
                    var downloadRequest = URLRequest(url: url)
                    let cookies = await store.webView.configuration.websiteDataStore.httpCookieStore.allCookies()
                    let host = url.host?.lowercased() ?? ""
                    let matches = cookies.filter { cookie in
                        let domain = cookie.domain.trimmingCharacters(in: CharacterSet(charactersIn: ".")).lowercased()
                        return (host == domain || host.hasSuffix("." + domain)) && url.path.hasPrefix(cookie.path) && (!cookie.isSecure || url.scheme == "https") && (cookie.expiresDate == nil || cookie.expiresDate! > Date())
                    }
                    for (key, value) in HTTPCookie.requestHeaderFields(with: matches) { downloadRequest.setValue(value, forHTTPHeaderField: key) }
                    if let headers = request["headers"] as? [String: String], let password = headers["x-share-password"] ?? headers["X-Share-Password"] {
                        guard !password.contains("\r"), !password.contains("\n") else { throw URLError(.badURL) }
                        downloadRequest.setValue(password, forHTTPHeaderField: "X-Share-Password")
                    }
                    let configuration = URLSessionConfiguration.ephemeral; configuration.httpCookieStorage = nil; configuration.timeoutIntervalForRequest = 120; configuration.timeoutIntervalForResource = 10800
                    let session = URLSession(configuration: configuration, delegate: DownloadRedirectPolicy(server: server), delegateQueue: nil)
                    defer { session.finishTasksAndInvalidate() }
                    let (temporary, response) = try await session.download(for: downloadRequest)
                    guard let response = response as? HTTPURLResponse, (200..<300).contains(response.statusCode) else { throw URLError(.badServerResponse) }
                    let folder = FileManager.default.temporaryDirectory.appendingPathComponent("TeldriveDownloads", isDirectory: true).appendingPathComponent(UUID().uuidString, isDirectory: true)
                    try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
                    let name = URL(fileURLWithPath: request["filename"] as? String ?? "archivo").lastPathComponent
                    let destination = folder.appendingPathComponent(name.isEmpty ? "archivo" : name)
                    try FileManager.default.moveItem(at: temporary, to: destination)
                    store.downloaded = DownloadedFile(url: destination); store.downloading = false; replyHandler(true, nil)
                } catch { store.downloading = false; replyHandler(nil, "No se pudo descargar el archivo. Comprueba el enlace, la contraseña y la conexión.") }
            }
        }

        func webView(_ view: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
            guard let url = action.request.url, let server = store.server else { decisionHandler(.cancel); return }
            let ownBlob = url.scheme == "blob" && url.absoluteString.hasPrefix("blob:" + String(server.absoluteString.dropLast()) + "/")
            if ServerAddress.sameOrigin(server, url) || ownBlob {
                decisionHandler(action.shouldPerformDownload ? .download : .allow)
            } else {
                decisionHandler(.cancel)
                if action.navigationType == .linkActivated && ["https", "http", "tg"].contains(url.scheme ?? "") { UIApplication.shared.open(url) }
            }
        }
        func webView(_ view: WKWebView, decidePolicyFor response: WKNavigationResponse, decisionHandler: @escaping (WKNavigationResponsePolicy) -> Void) {
            if let http = response.response as? HTTPURLResponse, let disposition = http.value(forHTTPHeaderField: "Content-Disposition"), disposition.lowercased().contains("attachment") { decisionHandler(.download) }
            else { decisionHandler(response.canShowMIMEType ? .allow : .download) }
        }
        func webView(_ view: WKWebView, navigationAction: WKNavigationAction, didBecome download: WKDownload) { begin(download) }
        func webView(_ view: WKWebView, navigationResponse: WKNavigationResponse, didBecome download: WKDownload) { begin(download) }
        private func begin(_ download: WKDownload) { download.delegate = self; store.downloading = true }

        func webView(_ view: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for action: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
            if let url = action.request.url, let server = store.server, ServerAddress.sameOrigin(server, url) { view.load(action.request) }
            else if let url = action.request.url, action.navigationType == .linkActivated, ["https", "http", "tg"].contains(url.scheme ?? "") { UIApplication.shared.open(url) }
            return nil
        }
        func webView(_ view: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) { report(error) }
        func webView(_ view: WKWebView, didFail navigation: WKNavigation!, withError error: Error) { report(error) }
        func webViewWebContentProcessDidTerminate(_ view: WKWebView) { store.error = "La vista se cerró. Pulsa Reintentar para abrir tu unidad de nuevo." }
        private func report(_ error: Error) {
            if (error as NSError).code == NSURLErrorCancelled { return }
            store.error = "No se pudo conectar al servidor. Comprueba la dirección, el certificado HTTPS y tu conexión."
        }
        func download(_ download: WKDownload, decideDestinationUsing response: URLResponse, suggestedFilename: String, completionHandler: @escaping (URL?) -> Void) {
            do {
                let folder = FileManager.default.temporaryDirectory.appendingPathComponent("TeldriveDownloads", isDirectory: true).appendingPathComponent(UUID().uuidString, isDirectory: true)
                try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
                let name = URL(fileURLWithPath: suggestedFilename).lastPathComponent
                let target = folder.appendingPathComponent(name.isEmpty ? "archivo" : name)
                destinations[ObjectIdentifier(download)] = target
                completionHandler(target)
            } catch { store.downloading = false; store.error = "No se pudo preparar la descarga en este dispositivo."; completionHandler(nil) }
        }
        func downloadDidFinish(_ download: WKDownload) {
            store.downloading = false
            if let url = destinations.removeValue(forKey: ObjectIdentifier(download)) { store.downloaded = DownloadedFile(url: url) }
        }
        func download(_ download: WKDownload, didFailWithError error: Error, resumeData: Data?) {
            store.downloading = false
            if let url = destinations.removeValue(forKey: ObjectIdentifier(download)) { try? FileManager.default.removeItem(at: url.deletingLastPathComponent()) }
            store.error = "La descarga no pudo completarse. Vuelve a intentarlo desde el archivo."
        }
    }
}

private final class DownloadRedirectPolicy: NSObject, URLSessionTaskDelegate {
    let server: URL
    init(server: URL) { self.server = server }
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse, newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) { completionHandler(request.url.map { ServerAddress.sameOrigin(server, $0) } == true ? request : nil) }
}
