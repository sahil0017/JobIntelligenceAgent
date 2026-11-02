import { test } from "@playwright/test";
test("dump", async ({ page }) => {
  await page.goto("/");
  for (const name of ["Overview","Job matches","Area job search","Applications","My profile","Agent activity","Settings"]) {
    await page.getByRole("button", { name }).first().click();
    console.log("=====", name, "\n", (await page.locator("main").innerText()).slice(0, 1500));
  }
});
