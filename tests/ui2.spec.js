import { expect, test } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.route("**/public/css/output.css*", (route) => route.continue());
});

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
    await expect(page.locator("body")).toHaveCSS(
      "background-color", colorScheme === "light" ? "rgb(255, 255, 255)" : "rgb(16, 18, 22)"
    );
    await expect(page.locator(".app-sidebar")).toHaveCSS(
      "background-color", colorScheme === "light" ? "rgb(242, 244, 247)" : "rgb(34, 39, 47)"
    );
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

test("print layout leaves navigation out and uses light colors", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("darkMode", "true"));
  await page.goto(process.env.BASE_URL ?? "http://localhost:8080");
  await page.emulateMedia({ media: "print" });

  await expect(page.locator("html")).toHaveCSS("color-scheme", "light");
  await expect(page.locator(".app-sidebar")).toBeHidden();
  await expect(page.locator(".app-frame")).toHaveCSS("overflow", "visible");
  await expect(page.locator(".app-main")).toHaveCSS("overflow-y", "visible");
  await expect(page.locator(".ui2-matrix-cell").first()).toBeVisible();

  await page.goto(new URL("/risks", process.env.BASE_URL ?? "http://localhost:8080").href);
  await expect(page.locator(".ui2-panel:has(> form[role='search'])")).toBeHidden();
  await expect(page.locator(".ui2-table")).toBeVisible();
});

test("risk table scrolls without widening the page on mobile", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(new URL("/risks", process.env.BASE_URL ?? "http://localhost:8080").href);

  const widths = await page.evaluate(() => {
    const main = document.querySelector(".app-main");
    const table = document.querySelector(".ui2-table");
    return {
      main: main.clientWidth,
      mainScroll: main.scrollWidth,
      table: table.clientWidth,
      tableScroll: table.scrollWidth,
    };
  });
  expect(widths.mainScroll).toBeLessThanOrEqual(widths.main);
  expect(widths.tableScroll).toBeGreaterThan(widths.table);
});
