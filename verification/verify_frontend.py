from playwright.sync_api import sync_playwright
import os

def run():
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page()

        # Navigate to the local file
        cwd = os.getcwd()
        page.goto(f"file://{cwd}/index.html")

        # Check if the board is rendered (cells exist)
        # We need to wait for the board to render
        page.wait_for_selector(".cell")

        # Take a screenshot
        page.screenshot(path="verification/board_screenshot.png")

        # Verify that cells have correct classes based on bit-packed logic
        # For example, player 1 base should have class 'player1-base'
        # We can check specific cells if we know the initial state
        # Initial state: (0,0) is player 1 base, (9,9) is player 2 base (assuming 10x10)

        # Check cell (0,0)
        cell_0_0 = page.locator('.cell[data-row="0"][data-col="0"]')
        class_0_0 = cell_0_0.get_attribute("class")
        print(f"Cell (0,0) class: {class_0_0}")

        if "player1-base" in class_0_0:
            print("SUCCESS: Cell (0,0) correctly identified as player 1 base")
        else:
            print("FAILURE: Cell (0,0) should be player 1 base")

        # Check cell (9,9) - assuming 10x10 default
        rows_input = page.locator("#rows-input")
        rows = int(rows_input.input_value())
        cols_input = page.locator("#cols-input")
        cols = int(cols_input.input_value())

        last_row = rows - 1
        last_col = cols - 1

        cell_last = page.locator(f'.cell[data-row="{last_row}"][data-col="{last_col}"]')
        class_last = cell_last.get_attribute("class")
        print(f"Cell ({last_row},{last_col}) class: {class_last}")

        if "player2-base" in class_last:
            print("SUCCESS: Last cell correctly identified as player 2 base")
        else:
            print("FAILURE: Last cell should be player 2 base")

        browser.close()

if __name__ == "__main__":
    run()
