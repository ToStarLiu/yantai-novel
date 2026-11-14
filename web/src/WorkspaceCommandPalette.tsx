import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { createPortal } from "react-dom";
import { ArrowUpRight, Command, LoaderCircle, Search, X } from "lucide-react";
import { message } from "./api";
import "./workspaceCommandPalette.css";

export type WorkspaceCommand = {
  id: string;
  label: string;
  description?: string;
  group?: string;
  shortcut?: string;
  onSelect: () => void | Promise<void>;
};

type Props = {
  open: boolean;
  onClose: () => void;
  commands: WorkspaceCommand[];
};

const commandFocusable =
  'input:not([disabled]):not([type="hidden"]), textarea:not([disabled]), select:not([disabled]), button:not([disabled]), a[href], [tabindex]';

function isVisible(element: HTMLElement) {
  return (
    element.isConnected &&
    element.getClientRects().length > 0 &&
    !element.closest('[hidden], [inert], [aria-hidden="true"]')
  );
}

function finishCommandFocus(
  trigger: Element | null,
  retiredDialog: HTMLElement | null,
  priorDialogs: Set<Element>,
  handoff: boolean,
) {
  let observer: MutationObserver | undefined;
  let timeout: ReturnType<typeof setTimeout> | undefined;
  let cancelled = false;
  const stop = () => {
    cancelled = true;
    observer?.disconnect();
    if (timeout) clearTimeout(timeout);
    cancelAnimationFrame(frame);
  };
  const focusDestination = () => {
    if (cancelled) return true;
    const active = document.activeElement;
    const destination = [
      ...document.querySelectorAll<HTMLElement>('[role="dialog"]'),
    ]
      .reverse()
      .find(
        (element) =>
          element !== retiredDialog &&
          !priorDialogs.has(element) &&
          isVisible(element),
      );
    if (destination?.contains(active)) {
      stop();
      return true;
    }
    // Respect focus the author (or a destination component) already moved.
    if (
      active &&
      active !== document.body &&
      active !== trigger &&
      active.isConnected &&
      !retiredDialog?.contains(active)
    ) {
      stop();
      return true;
    }
    if (!destination) return false;
    const control = [
      ...destination.querySelectorAll<HTMLElement>(commandFocusable),
    ]
      .filter((element) => element.tabIndex >= 0 && isVisible(element))
      .find((element) => !element.closest('[aria-disabled="true"]'));
    if (control) control.focus({ preventScroll: true });
    else {
      destination.tabIndex = -1;
      destination.focus({ preventScroll: true });
    }
    stop();
    return true;
  };
  const frame = requestAnimationFrame(() => {
    if (focusDestination()) return;
    if (trigger instanceof HTMLElement && isVisible(trigger)) {
      trigger.focus({ preventScroll: true });
    }
    if (!handoff) {
      stop();
      return;
    }
    // Lazy tools can mount after this dialog has gone. Watch briefly for that
    // destination; ordinary navigation still restores the surviving trigger.
    observer = new MutationObserver(focusDestination);
    observer.observe(document.body, { childList: true, subtree: true });
    timeout = setTimeout(stop, 2000);
    focusDestination();
  });
  return stop;
}

// Mount a fresh dialog on each opening while keeping the caller's draft and
// navigation state outside this component. It only dispatches provided actions.
export default function WorkspaceCommandPalette(props: Props) {
  if (!props.open) return null;
  return createPortal(
    <CommandDialog commands={props.commands} onClose={props.onClose} />,
    document.body,
  );
}

function CommandDialog({ commands, onClose }: Omit<Props, "open">) {
  const dialog = useRef<HTMLDivElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const callback = useRef(onClose);
  const executing = useRef(false);
  const completed = useRef(false);
  const cancelFocusReturn = useRef<(() => void) | null>(null);
  const returnFocusTrigger = useRef<Element | null | undefined>(undefined);
  const mounted = useRef(true);
  const baseId = useId();
  const [query, setQuery] = useState("");
  const [activeId, setActiveId] = useState("");
  const [running, setRunning] = useState("");
  const [error, setError] = useState("");
  callback.current = onClose;

  const filtered = useMemo(() => {
    const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean);
    const matching = commands.filter((command) => {
      const text = [command.label, command.description, command.group]
        .filter(Boolean)
        .join(" ")
        .toLocaleLowerCase();
      return terms.every((term) => text.includes(term));
    });
    // A group stays together even if the caller interleaves its actions.
    const groups = new Map<string, WorkspaceCommand[]>();
    for (const command of matching) {
      const group = command.group || "常用操作";
      groups.set(group, [...(groups.get(group) || []), command]);
    }
    return [...groups.entries()];
  }, [commands, query]);
  const options = filtered.flatMap(([, items]) => items);
  const selected =
    options.find((command) => command.id === activeId) || options[0];
  const selectedIndex = selected
    ? options.findIndex((command) => command.id === selected.id)
    : -1;
  const optionId = (index: number) => `${baseId}-option-${index}`;

  useEffect(() => {
    cancelFocusReturn.current?.();
    mounted.current = true;
    if (returnFocusTrigger.current === undefined)
      returnFocusTrigger.current = document.activeElement;
    const trigger = returnFocusTrigger.current;
    const palette = dialog.current;
    const priorDialogs = new Set(document.querySelectorAll('[role="dialog"]'));
    search.current?.focus();
    const containFocus = (event: FocusEvent) => {
      const destination =
        event.target instanceof Element
          ? event.target.closest('[role="dialog"]')
          : null;
      if (executing.current && destination && !priorDialogs.has(destination))
        return;
      if (dialog.current && !dialog.current.contains(event.target as Node)) {
        search.current?.focus({ preventScroll: true });
      }
    };
    document.addEventListener("focusin", containFocus);
    return () => {
      mounted.current = false;
      document.removeEventListener("focusin", containFocus);
      cancelFocusReturn.current = finishCommandFocus(
        trigger,
        palette,
        priorDialogs,
        completed.current || executing.current,
      );
    };
  }, []);

  useEffect(() => {
    if (!selected) return;
    list.current
      ?.querySelector<HTMLElement>(`[data-command-index="${selectedIndex}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [selected?.id, selectedIndex]);

  const close = () => {
    if (!executing.current) callback.current();
  };
  const execute = async (command: WorkspaceCommand) => {
    if (executing.current) return;
    executing.current = true;
    setRunning(command.id);
    setError("");
    try {
      await command.onSelect();
      completed.current = true;
      if (mounted.current) callback.current();
    } catch (cause) {
      if (mounted.current) {
        setError(message(cause) || "操作未完成，请重试。");
        search.current?.focus({ preventScroll: true });
      }
    } finally {
      executing.current = false;
      if (mounted.current) setRunning("");
    }
  };
  const handleKey = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.nativeEvent.isComposing) return;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close();
    } else if (event.key === "Tab") {
      const focusable = dialog.current?.querySelectorAll<HTMLElement>(
        'input:not([disabled]), button:not([disabled]):not([tabindex="-1"]), [tabindex="0"]',
      );
      if (!focusable?.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    } else if (
      document.activeElement === search.current &&
      !executing.current
    ) {
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        if (!options.length) return;
        const step = event.key === "ArrowDown" ? 1 : -1;
        setActiveId(
          options[(selectedIndex + step + options.length) % options.length].id,
        );
      } else if (event.key === "Enter" && selected) {
        event.preventDefault();
        void execute(selected);
      }
    }
  };

  return (
    <div
      className="workspace-command-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) close();
      }}
    >
      <div
        className="workspace-command-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${baseId}-title`}
        aria-describedby={`${baseId}-help`}
        aria-busy={!!running}
        ref={dialog}
        onKeyDown={handleKey}
      >
        <header className="workspace-command-heading">
          <span>
            <Command size={16} />
            <strong id={`${baseId}-title`}>快捷操作</strong>
          </span>
          <button
            type="button"
            className="workspace-command-close"
            aria-label="关闭快捷操作"
            onClick={close}
            disabled={!!running}
          >
            <X size={17} />
          </button>
        </header>
        <div className="workspace-command-search">
          <Search size={19} aria-hidden="true" />
          <input
            ref={search}
            role="combobox"
            aria-label="搜索快捷操作"
            aria-controls={`${baseId}-list`}
            aria-expanded="true"
            aria-autocomplete="list"
            aria-activedescendant={
              selectedIndex >= 0 ? optionId(selectedIndex) : undefined
            }
            placeholder="搜索章节、规划、人物、写作工具…"
            value={query}
            readOnly={!!running}
            onChange={(event) => {
              setQuery(event.target.value);
              setActiveId("");
            }}
          />
          <kbd>Esc</kbd>
        </div>
        <div
          ref={list}
          className="workspace-command-list"
          id={`${baseId}-list`}
          role="listbox"
          aria-label="可用快捷操作"
          aria-disabled={!!running}
        >
          {filtered.length ? (
            filtered.map(([group, items], groupIndex) => (
              <section
                key={group}
                role="group"
                aria-labelledby={`${baseId}-group-${groupIndex}`}
                className="workspace-command-group"
              >
                <h3 id={`${baseId}-group-${groupIndex}`}>{group}</h3>
                {items.map((command) => {
                  const index = options.findIndex(
                    (item) => item.id === command.id,
                  );
                  return (
                    <button
                      key={command.id}
                      id={optionId(index)}
                      type="button"
                      role="option"
                      aria-selected={selected?.id === command.id}
                      tabIndex={-1}
                      data-command-index={index}
                      className="workspace-command-option"
                      disabled={!!running}
                      onMouseMove={() => {
                        if (!executing.current) setActiveId(command.id);
                      }}
                      onMouseDown={(event) => event.preventDefault()}
                      onClick={() => void execute(command)}
                    >
                      <span className="workspace-command-option-copy">
                        <strong>{command.label}</strong>
                        {command.description && (
                          <span>{command.description}</span>
                        )}
                      </span>
                      {running === command.id ? (
                        <LoaderCircle
                          size={17}
                          className="workspace-command-spinner"
                          aria-label="正在执行"
                        />
                      ) : command.shortcut ? (
                        <kbd>{command.shortcut}</kbd>
                      ) : (
                        <ArrowUpRight size={15} aria-hidden="true" />
                      )}
                    </button>
                  );
                })}
              </section>
            ))
          ) : (
            <div className="workspace-command-empty" role="status">
              <Search size={22} />
              <strong>没有匹配的操作</strong>
              <span>试试“章节”“规划”或“人物”。</span>
            </div>
          )}
        </div>
        {error && (
          <div className="workspace-command-error" role="alert">
            {error}
          </div>
        )}
        <footer className="workspace-command-footer" id={`${baseId}-help`}>
          <span>
            <kbd>↑</kbd> <kbd>↓</kbd> 选择 <kbd>Enter</kbd> 打开
          </span>
          <span role="status" aria-live="polite">
            {running ? "正在完成操作，请稍候…" : `${options.length} 项操作`}
          </span>
        </footer>
      </div>
    </div>
  );
}
