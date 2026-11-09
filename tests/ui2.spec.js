import { expect, test } from "@playwright/test";

for (const colorScheme of ["light", "dark"]) {
  test(`risk heat map and labels remain colored in ${colorScheme} mode`, async ({ page }) => {
    await page.emulateMedia({ colorScheme });
    await page.addInitScript((dark) => {
      localStorage.setItem("darkMode", String(dark));
    }, colorScheme === "dark");
    await page.goto(process.env.BASE_URL ?? "http://localhost:8080");
    await expect(page.locator(".ui2-dashboard")).toBeVisible();
    const cells = page.locator(".ui2-matrix-cell");
    await expect(cells).toHaveCount(25);
    await page.screenshot({ path: test.info().outputPath(`dashboard-${colorScheme}.png`), fullPage: true });
    await page.locator(".ui2-matrix").screenshot({ path: test.info().outputPath(`heatmap-${colorScheme}.png`) });

    await expect(page.locator("body")).toHaveCSS("font-family", /system-ui.*sans-serif/);
    await expect(page.locator("html")).toHaveCSS("color-scheme", colorScheme);
    for (const tone of ["success", "warning", "danger"]) {
      const background = await page.locator(`.ui2-matrix-cell[data-tone="${tone}"]`).first()
        .evaluate((element) => getComputedStyle(element).backgroundColor);
      const [red, green, blue] = background.match(/\d+/g).map(Number);
      if (tone === "success") expect(green).toBeGreaterThan(red);
      if (tone === "danger") expect(red).toBeGreaterThan(green);
      if (tone === "warning") {
        expect(red).toBeGreaterThan(blue);
        expect(green).toBeGreaterThan(blue);
      }
      await expect(page.locator(`.ui2-legend .ui2-badge[data-tone="${tone}"]`).first())
        .toHaveCSS("background-color", background);
    }
  });
}
