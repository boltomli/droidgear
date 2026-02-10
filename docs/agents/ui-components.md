# UI Components

Guidelines for working with UI components in this project.

## Radix UI Focus Management

When using Radix UI components (Dialog, DropdownMenu, Popover, etc.), be aware of automatic focus behavior.

**Problem**: Radix components return focus to the trigger element when closed, which may override manual `focus()` calls.

**Solution**: Use `onCloseAutoFocus` to prevent default behavior and manually control focus:

```tsx
<DropdownMenuContent
  onCloseAutoFocus={e => {
    e.preventDefault()
    targetRef.current?.focus()
  }}
>
```

This applies to: `DialogContent`, `DropdownMenuContent`, `PopoverContent`, `AlertDialogContent`, etc.

## ResizableDialog

Always pass `onCloseAutoFocus` to `ResizableDialogContent` to prevent double-click-to-close issues caused by focus returning to the drag handle:

```tsx
<ResizableDialogContent
  onCloseAutoFocus={e => {
    e.preventDefault()
  }}
>
```

## IME (Input Method Editor) Compatibility

When dialogs contain input fields, always use `onCloseAutoFocus={e => e.preventDefault()}` to prevent issues with Chinese/Japanese/Korean IME.

Without this, clicking Cancel while IME is active requires two clicks: first to commit IME composition, second to actually close the dialog. This is because Radix's default focus return behavior interferes with IME blur events.

## Further Reading

- `docs/developer/ui-patterns.md` — CSS architecture, shadcn/ui components, Tailwind v4 configuration
