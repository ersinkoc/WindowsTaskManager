import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "./confirm-dialog";

describe("ConfirmDialog", () => {
  let originalOverflow: string;

  beforeEach(() => {
    originalOverflow = document.body.style.overflow;
  });

  afterEach(() => {
    document.body.style.overflow = originalOverflow;
  });

  it("renders nothing when open=false", () => {
    const { container } = render(
      <ConfirmDialog
        open={false}
        title="Are you sure?"
        description="This is permanent."
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders the title and description when open=true", () => {
    render(
      <ConfirmDialog
        open
        title="Delete this thing?"
        description="This action cannot be undone."
        confirmLabel="Delete"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    expect(screen.getByText("Delete this thing?")).toBeInTheDocument();
    expect(screen.getByText("This action cannot be undone.")).toBeInTheDocument();
  });

  it("uses 'Cancel' as the default cancel label", () => {
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("uses the provided cancelLabel when given", () => {
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        cancelLabel="Nevermind"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    expect(screen.getByRole("button", { name: "Nevermind" })).toBeInTheDocument();
  });

  it("uses 'danger' variant for the confirm button by default", () => {
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    // Danger buttons get a red border class via the --error CSS var.
    const confirmButton = screen.getByRole("button", { name: "Confirm" });
    expect(confirmButton.className).toContain("border-error");
  });

  it("uses 'primary' variant for the confirm button when tone='neutral'", () => {
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="OK"
        tone="neutral"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    const confirmButton = screen.getByRole("button", { name: "OK" });
    expect(confirmButton.className).toContain("border-accent");
    expect(confirmButton.className).not.toContain("border-error");
  });

  it("renders the confirm label when not pending", () => {
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Delete now"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    expect(screen.getByRole("button", { name: "Delete now" })).toBeInTheDocument();
    expect(screen.queryByText("Working...")).toBeNull();
  });

  it("renders a spinner with the 'Working...' label when isPending=true", () => {
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Delete"
        isPending
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    expect(screen.getByText("Working...")).toBeInTheDocument();
    // The lucide LoaderCircle icon must be rendered.
    const confirmButton = screen.getByRole("button", { name: /Working\.\.\./ });
    expect(confirmButton.querySelector("svg")).not.toBeNull();
  });

  it("disables all action buttons while isPending=true", () => {
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Delete"
        isPending
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    expect(screen.getByRole("button", { name: /Working\.\.\./ })).toBeDisabled();
    // The backdrop close button must also be disabled.
    const backdrop = screen.getByRole("button", { name: "Close confirmation dialog" });
    expect(backdrop).toBeDisabled();
  });

  it("calls onConfirm when the confirm button is clicked", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={onConfirm}
        onOpenChange={() => undefined}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("calls onOpenChange(false) when the cancel button is clicked", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={onOpenChange}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("calls onOpenChange(false) when the backdrop is clicked", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={onOpenChange}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Close confirmation dialog" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("does not close when the backdrop is clicked while isPending=true", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        isPending
        onConfirm={() => undefined}
        onOpenChange={onOpenChange}
      />,
    );
    const backdrop = screen.getByRole("button", { name: "Close confirmation dialog" });
    // The button is disabled — clicking a disabled button is a no-op.
    await user.click(backdrop);
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("closes via the Escape key when not pending", () => {
    const onOpenChange = vi.fn();
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={onOpenChange}
      />,
    );
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("does NOT close via the Escape key when isPending=true", () => {
    const onOpenChange = vi.fn();
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        isPending
        onConfirm={() => undefined}
        onOpenChange={onOpenChange}
      />,
    );
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("ignores non-Escape keydowns", () => {
    const onOpenChange = vi.fn();
    render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={onOpenChange}
      />,
    );
    fireEvent.keyDown(window, { key: "Enter" });
    fireEvent.keyDown(window, { key: "a" });
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("locks body scroll when open and restores it on close", () => {
    document.body.style.overflow = "auto";
    const { rerender } = render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    expect(document.body.style.overflow).toBe("hidden");

    rerender(
      <ConfirmDialog
        open={false}
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    expect(document.body.style.overflow).toBe("auto");
  });

  it("does not register a keydown listener when not open", () => {
    const onOpenChange = vi.fn();
    render(
      <ConfirmDialog
        open={false}
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={onOpenChange}
      />,
    );
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("removes the keydown listener on unmount", () => {
    const onOpenChange = vi.fn();
    const { unmount } = render(
      <ConfirmDialog
        open
        title="t"
        description="d"
        confirmLabel="Confirm"
        onConfirm={() => undefined}
        onOpenChange={onOpenChange}
      />,
    );
    unmount();
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("renders into document.body via a portal", async () => {
    const { container } = render(
      <ConfirmDialog
        open
        title="Portal title"
        description="Portal description"
        confirmLabel="Go"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    await waitFor(() => {
      // The dialog lives in document.body, NOT in the React testing-library container.
      expect(document.body.querySelector('[role="alertdialog"]')).not.toBeNull();
    });
    // And the React render container should not have a dialog inside it.
    expect(container.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it("links the dialog to its title and description via aria-labelledby/aria-describedby", () => {
    render(
      <ConfirmDialog
        open
        title="Linked title"
        description="Linked description"
        confirmLabel="OK"
        onConfirm={() => undefined}
        onOpenChange={() => undefined}
      />,
    );
    const dialog = screen.getByRole("alertdialog");
    const labelledBy = dialog.getAttribute("aria-labelledby");
    const describedBy = dialog.getAttribute("aria-describedby");
    expect(labelledBy).not.toBeNull();
    expect(describedBy).not.toBeNull();
    if (!labelledBy || !describedBy) return;
    const titleEl = document.getElementById(labelledBy);
    const descEl = document.getElementById(describedBy);
    expect(titleEl).toHaveTextContent("Linked title");
    expect(descEl).toHaveTextContent("Linked description");
  });

  it("uses stable per-instance IDs even when multiple dialogs are mounted", () => {
    render(
      <>
        <ConfirmDialog
          open
          title="First"
          description="First desc"
          confirmLabel="A"
          onConfirm={() => undefined}
          onOpenChange={() => undefined}
        />
        <ConfirmDialog
          open
          title="Second"
          description="Second desc"
          confirmLabel="B"
          onConfirm={() => undefined}
          onOpenChange={() => undefined}
        />
      </>,
    );
    const dialogs = document.body.querySelectorAll('[role="alertdialog"]');
    expect(dialogs).toHaveLength(2);
    const first = dialogs[0] as HTMLElement;
    const second = dialogs[1] as HTMLElement;
    expect(first.getAttribute("aria-labelledby")).not.toBe(second.getAttribute("aria-labelledby"));
    expect(first.getAttribute("aria-describedby")).not.toBe(second.getAttribute("aria-describedby"));
  });
});
