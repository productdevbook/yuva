import { afterEach, beforeAll, describe, expect, test } from "bun:test";
import { define, YuvaChatElement } from "../src/element";
import { directionOf, resolveLocale } from "../src/i18n";

let loads = 0;

beforeAll(() => {
  YuvaChatElement.loadPanel = () => {
    loads++;
    return import("../src/yuva-chat");
  };
  define();
});

afterEach(() => {
  document.body.replaceChildren();
  loads = 0;
});

function mount(attributes: Record<string, string>): YuvaChatElement {
  const element = document.createElement("yuva-chat") as YuvaChatElement;
  for (const [name, value] of Object.entries(attributes)) element.setAttribute(name, value);
  document.body.append(element);
  return element;
}

const shadow = (element: YuvaChatElement) => element.shadowRoot!;
const panel = (element: YuvaChatElement) => shadow(element).querySelector<HTMLElement>(".panel")!;
const launcher = (element: YuvaChatElement) => shadow(element).querySelector<HTMLButtonElement>(".launcher");
const until = async (condition: () => boolean) => {
  for (let i = 0; i < 50 && !condition(); i++) await new Promise((resolve) => setTimeout(resolve, 5));
};

describe("<yuva-chat> launcher", () => {
  test("renders a closed launcher without loading the panel", () => {
    const element = mount({ inbox: "inb_1", locale: "en" });
    expect(launcher(element)?.getAttribute("aria-label")).toBe("Open chat");
    expect(launcher(element)?.getAttribute("aria-expanded")).toBe("false");
    expect(panel(element).hidden).toBe(true);
    expect(loads).toBe(0);
  });

  test("opens and closes, loading the panel once", async () => {
    const element = mount({ inbox: "inb_1", locale: "en" });
    launcher(element)!.click();
    await until(() => !panel(element).hidden);
    expect(element.isOpen).toBe(true);
    expect(panel(element).querySelector(".title")?.textContent).toBe("Messages");
    expect(launcher(element)?.getAttribute("aria-label")).toBe("Close chat");

    panel(element).querySelector<HTMLButtonElement>(".close")!.click();
    expect(element.isOpen).toBe(false);
    expect(panel(element).hidden).toBe(true);

    await element.open();
    expect(panel(element).hidden).toBe(false);
    expect(loads).toBe(1);
  });

  test("translates to Turkish and follows locale changes", async () => {
    const element = mount({ locale: "tr-TR" });
    expect(launcher(element)?.getAttribute("aria-label")).toBe("Sohbeti aç");
    await element.open();
    expect(panel(element).querySelector(".title")?.textContent).toBe("Mesajlar");

    element.setAttribute("locale", "en");
    await until(() => panel(element).querySelector(".title")?.textContent === "Messages");
    expect(panel(element).querySelector(".title")?.textContent).toBe("Messages");
    expect(launcher(element)?.getAttribute("aria-label")).toBe("Close chat");
  });

  test("closes on Escape", async () => {
    const element = mount({ locale: "en" });
    await element.open();
    panel(element).dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    expect(element.isOpen).toBe(false);
  });
});

describe("<yuva-chat> embedded", () => {
  test("shows the panel without a launcher or close button", async () => {
    const element = mount({ layout: "embedded", locale: "tr" });
    await until(() => !panel(element).hidden);
    expect(launcher(element)).toBeNull();
    expect(panel(element).querySelector(".close")).toBeNull();
    expect(panel(element).querySelector(".empty")?.textContent).toBe("Henüz konuşma yok");
    element.close();
    expect(panel(element).hidden).toBe(false);
  });
});

describe("locale", () => {
  test("resolves to a supported locale", () => {
    expect(resolveLocale("tr-TR")).toBe("tr");
    expect(resolveLocale("de", "tr")).toBe("tr");
    expect(resolveLocale("de", null)).toBe("en");
  });

  test("knows text direction", () => {
    expect(directionOf("ar-EG")).toBe("rtl");
    expect(directionOf("tr")).toBe("ltr");
  });
});
