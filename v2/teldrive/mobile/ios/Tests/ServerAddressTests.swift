import XCTest
@testable import Teldrive

final class ServerAddressTests: XCTestCase {
    func testAllowsHTTPSAndExplicitPrivateHTTP() throws {
        XCTAssertEqual(try ServerAddress.parse("https://example.com", allowLocalHTTP: false).absoluteString, "https://example.com/")
        XCTAssertEqual(try ServerAddress.parse("http://192.168.1.20:8080", allowLocalHTTP: true).port, 8080)
        XCTAssertTrue(ServerAddress.sameOrigin(URL(string:"https://example.com")!, URL(string:"https://example.com:443/files")!))
    }
    func testRejectsCredentialsPathsPublicHTTPAndMalformedPrivateHosts() {
        for value in ["http://example.com", "https://user:password@example.com", "https://example.com/files", "https://example.com?key=secret", "https://example.com:0", "http://10.0.foo.0.1", "http://10.0.0.1."] {
            XCTAssertThrowsError(try ServerAddress.parse(value, allowLocalHTTP:true), value)
        }
        XCTAssertThrowsError(try ServerAddress.parse("http://192.168.1.20", allowLocalHTTP:false))
        XCTAssertFalse(ServerAddress.sameOrigin(URL(string:"https://example.com")!, URL(string:"https://example.com:8443/files")!))
    }
}
