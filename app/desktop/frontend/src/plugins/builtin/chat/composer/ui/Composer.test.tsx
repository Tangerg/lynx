import type { ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { COMPOSER_KEY_BINDING } from "@/plugins/sdk/kernelPoints";
import { addLocaleBundle, setLocale } from "@/lib/i18n";
import { WORKSPACE_LIST_FILES_KEY } from "@/plugins/builtin/workspace/public/queries";
import { Composer } from "./Composer";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => true,
}));

// Composer now reads the workspace file list (useFileMentions → useListFiles)
// via React Query, so renders need a provider. Retries off so a missing
// provider/fetcher fails fast rather than hanging the test.
function wrap(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
    },
  });
  // These Composer cases do not open a mention. Seed the disabled query's
  // identity without inventing a Runtime provider or coupling this component
  // spec to a transport adapter.
  client.setQueryData([WORKSPACE_LIST_FILES_KEY, undefined], []);
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

// Composer relies on a built-in composer-keymap registration to bind
// Enter → submit. Set up a tiny in-test plugin that mirrors it.
async function withEnterKeymap() {
  await loadPluginsForTest(
    definePlugin({
      name: "test.composer-keymap",
      setup: (ctx) => {
        ctx.contribute(COMPOSER_KEY_BINDING, {
          key: "Enter",
          description: "submit",
          handler: ({ submit }) => {
            submit();
            return true;
          },
        });
      },
    }),
  );
}

const baseProps = {
  onClear: () => {},
  images: [],
  onRemoveImage: () => {},
  onAddImages: () => {},
  pastes: [],
  onRemovePaste: () => {},
  onAddPaste: () => {},
  acceptsImages: true,
  mode: "agent" as const,
  onModeChange: () => {},
};

afterEach(async () => {
  await act(async () => {
    setLocale("en");
    await Promise.resolve();
  });
});

describe("composer", () => {
  it("does not draw an internal divider for a standing surface that renders nothing", async () => {
    await loadPluginsForTest(
      definePlugin({
        name: "test.empty-composer-standing-surface",
        setup: (ctx) => {
          contributeLayout(ctx, "composer.header", {
            id: "empty",
            component: () => null,
          });
        },
      }),
    );

    const { container } = wrap(
      <Composer {...baseProps} value="" onChange={() => {}} onSend={() => true} />,
    );

    expect(container.querySelector(".border-b")).toBeNull();
  });

  it("calls onChange as the user types", () => {
    const onChange = vi.fn();
    wrap(<Composer {...baseProps} value="" onChange={onChange} onSend={() => true} />);
    const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "hi" } });
    expect(onChange).toHaveBeenCalledWith("hi");
  });

  it("does not reference an unmounted mention listbox", () => {
    wrap(<Composer {...baseProps} value="" onChange={() => {}} onSend={() => true} />);

    expect(screen.getByRole("textbox").hasAttribute("aria-controls")).toBe(false);
  });

  it("submits non-empty text on Enter when a binding maps Enter → submit", async () => {
    await withEnterKeymap();
    const onSend = vi.fn(() => true);
    const onChange = vi.fn();
    wrap(<Composer {...baseProps} value="hello world" onChange={onChange} onSend={onSend} />);
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
    expect(onSend).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "hello world" }] });
  });

  it("keeps the Enter that immediately follows a WebKit IME composition end inside the composer", async () => {
    await withEnterKeymap();
    const onSend = vi.fn(() => true);
    wrap(<Composer {...baseProps} value="english" onChange={() => {}} onSend={onSend} />);
    const textarea = screen.getByRole("textbox");

    // WKWebView can end a Chinese-IME composition before dispatching the
    // keydown from the same physical Enter. Both composing flags are false by
    // the time the key binding sees that keydown; keyCode 229 is the remaining
    // browser signal that this is still the IME's commit key.
    fireEvent.compositionStart(textarea, { data: "englis" });
    fireEvent.compositionEnd(textarea, { data: "english" });
    fireEvent.keyDown(textarea, { key: "Enter", keyCode: 229, isComposing: false });

    expect(onSend).not.toHaveBeenCalled();
  });

  it("keeps an unmarked Enter that commits Latin text through a Chinese IME inside the composer", async () => {
    await withEnterKeymap();
    const onSend = vi.fn(() => true);
    wrap(<Composer {...baseProps} value="中文 english" onChange={() => {}} onSend={onSend} />);
    const textarea = screen.getByRole("textbox");

    // Some Chinese IMEs commit raw Latin text by ending composition first and
    // then emitting an ordinary keyCode=13 Enter from the same physical key.
    // There is no native composing bit or WebKit 229 marker left to inspect.
    fireEvent.compositionStart(textarea, { data: "english" });
    fireEvent.compositionEnd(textarea, { data: "english" });
    fireEvent.keyDown(textarea, { key: "Enter", keyCode: 13, isComposing: false });

    expect(onSend).not.toHaveBeenCalled();

    // The ownership must expire with that browser event turn; the user's next
    // deliberate Enter still goes through the configured composer keymap.
    await act(async () => Promise.resolve());
    fireEvent.keyDown(textarea, { key: "Enter", keyCode: 13, isComposing: false });
    expect(onSend).toHaveBeenCalledOnce();
  });

  it("recovers the same commit ownership when a browser drops compositionend", async () => {
    await withEnterKeymap();
    const onSend = vi.fn(() => true);
    wrap(<Composer {...baseProps} value="中文 english" onChange={() => {}} onSend={onSend} />);
    const textarea = screen.getByRole("textbox");

    fireEvent.compositionStart(textarea, { data: "english" });
    fireEvent.change(textarea, { target: { value: "中文 english" } });
    fireEvent.keyDown(textarea, { key: "Enter", keyCode: 13, isComposing: false });

    expect(onSend).not.toHaveBeenCalled();
  });

  it("does not submit when the textarea is empty / whitespace only", async () => {
    await withEnterKeymap();
    const onSend = vi.fn(() => true);
    wrap(<Composer {...baseProps} value="   " onChange={() => {}} onSend={onSend} />);
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
    expect(onSend).not.toHaveBeenCalled();
  });

  it("renders image thumbnails when images are staged", () => {
    wrap(
      <Composer
        {...baseProps}
        value=""
        onChange={() => {}}
        onSend={() => true}
        images={[
          { id: "a1", mime: "image/png", data: "AAAA", name: "shot1.png" },
          { id: "a2", mime: "image/jpeg", data: "BBBB", name: "shot2.jpg" },
        ]}
      />,
    );
    const imgs = screen.getAllByRole("img");
    expect(imgs).toHaveLength(2);
    expect(imgs[0]!.getAttribute("src")).toBe("data:image/png;base64,AAAA");
  });

  it("refreshes the placeholder when the locale changes", async () => {
    addLocaleBundle("placeholder-test", {
      "composer.input.label": "Prompt",
      "composer.placeholder": "Localized placeholder",
    });
    wrap(<Composer {...baseProps} value="" onChange={() => {}} onSend={() => true} />);

    await act(async () => {
      setLocale("placeholder-test");
      await Promise.resolve();
    });

    await waitFor(() => {
      const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
      expect(textarea.placeholder).toBe("Localized placeholder");
    });
  });
});
