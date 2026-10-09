import XCTest

final class ConnectionUITests: XCTestCase {
    func testConnectionFormHasWorkingTouchControlsAndRejectsPublicHTTP() {
        let app = XCUIApplication(); app.launch()
        let field = app.textFields["https://teldrive.tudominio.com"]
        XCTAssertTrue(field.waitForExistence(timeout:10))
        field.tap(); field.typeText("http://example.com")
        app.buttons["Conectar a mi unidad"].tap()
        XCTAssertTrue(app.staticTexts["Utiliza HTTPS. Puedes permitir HTTP para una dirección privada de tu red local."].waitForExistence(timeout:5))
        app.switches["Permitir HTTP en mi red local"].tap()
        app.buttons["Conectar a mi unidad"].tap()
        XCTAssertTrue(field.exists)
    }
}
