const focusableSelector = [
    'a[href]',
    'button:not([disabled])',
    'input:not([disabled]):not([type="hidden"])',
    'select:not([disabled])',
    'textarea:not([disabled])',
    '[tabindex]:not([tabindex="-1"])'
].join(',');

const modalStack = [];
let nextModalId = 0;

function syncModalStack() {
    const top = modalStack.at(-1);
    modalStack.forEach(controller => {
        if (controller === top) {
            controller.overlay.inert = false;
            controller.overlay.removeAttribute('aria-hidden');
            controller.dialog.setAttribute('aria-modal', 'true');
        } else {
            controller.overlay.inert = true;
            controller.overlay.setAttribute('aria-hidden', 'true');
            controller.dialog.removeAttribute('aria-modal');
        }
    });
}

export function activateModal(overlay, { initialFocus, onClose } = {}) {
    const dialog = overlay.querySelector('.modal');
    if (!dialog) {
        throw new Error('Modal markup must contain an element with class "modal"');
    }

    const previousFocus = document.activeElement;
    const title = dialog.querySelector('.modal-title');
    dialog.setAttribute('role', 'dialog');
    dialog.tabIndex = -1;
    if (title) {
        title.id ||= `modal-title-${++nextModalId}`;
        dialog.setAttribute('aria-labelledby', title.id);
    }

    const controller = {
        overlay,
        dialog,
        close() {
            const index = modalStack.indexOf(controller);
            if (index === -1) {
                return;
            }
            overlay.removeEventListener('keydown', handleKeydown);
            overlay.removeEventListener('mousedown', handleBackdrop);
            modalStack.splice(index, 1);
            overlay.remove();
            syncModalStack();
            document.body.classList.toggle('modal-open', modalStack.length > 0);
            onClose?.();
            if (previousFocus?.isConnected) {
                previousFocus.focus();
            }
        }
    };

    function handleKeydown(event) {
        if (modalStack.at(-1) !== controller) {
            return;
        }
        if (event.key === 'Escape') {
            event.preventDefault();
            controller.close();
            return;
        }
        if (event.key !== 'Tab') {
            return;
        }
        const focusable = [...dialog.querySelectorAll(focusableSelector)];
        if (focusable.length === 0) {
            event.preventDefault();
            dialog.focus();
            return;
        }
        const first = focusable[0];
        const last = focusable.at(-1);
        if (event.shiftKey && document.activeElement === first) {
            event.preventDefault();
            last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault();
            first.focus();
        }
    }

    function handleBackdrop(event) {
        if (event.target === overlay && modalStack.at(-1) === controller) {
            controller.close();
        }
    }

    overlay.addEventListener('keydown', handleKeydown);
    overlay.addEventListener('mousedown', handleBackdrop);
    modalStack.push(controller);
    document.body.classList.add('modal-open');
    const preferredTarget = initialFocus ? dialog.querySelector(initialFocus) : null;
    const target = preferredTarget?.matches(focusableSelector)
        ? preferredTarget
        : dialog.querySelector(focusableSelector);
    if (modalStack.at(-1) === controller && overlay.isConnected) {
        (target || dialog).focus();
    }
    syncModalStack();
    return controller;
}

export function closeTopModal() {
    modalStack.at(-1)?.close();
}

export function closeAllModals() {
    while (modalStack.length > 0) {
        modalStack.at(-1).close();
    }
}

export function confirmDialog({ title, message, confirmLabel, cancelLabel, danger = false }) {
    const container = document.getElementById('modal-container');
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.innerHTML = `
        <div class="modal">
            <div class="modal-header">
                <h3 class="modal-title"></h3>
                <button class="modal-close" type="button" aria-label="${escapeAttribute(cancelLabel)}">×</button>
            </div>
            <div class="modal-body"><p class="dialog-message"></p></div>
            <div class="modal-footer">
                <button class="btn btn-secondary dialog-cancel" type="button"></button>
                <button class="btn ${danger ? 'btn-danger' : 'btn-primary'} dialog-confirm" type="button"></button>
            </div>
        </div>
    `;
    overlay.querySelector('.modal-title').textContent = title;
    overlay.querySelector('.dialog-message').textContent = message;
    overlay.querySelector('.dialog-cancel').textContent = cancelLabel;
    overlay.querySelector('.dialog-confirm').textContent = confirmLabel;
    container.appendChild(overlay);

    return new Promise(resolve => {
        let settled = false;
        const controller = activateModal(overlay, {
            initialFocus: '.dialog-cancel',
            onClose: () => {
                if (!settled) {
                    settled = true;
                    resolve(false);
                }
            }
        });
        const finish = value => {
            if (settled) {
                return;
            }
            settled = true;
            controller.close();
            resolve(value);
        };
        overlay.querySelector('.modal-close').addEventListener('click', () => finish(false));
        overlay.querySelector('.dialog-cancel').addEventListener('click', () => finish(false));
        overlay.querySelector('.dialog-confirm').addEventListener('click', () => finish(true));
    });
}

function escapeAttribute(value) {
    return String(value).replaceAll('&', '&amp;').replaceAll('"', '&quot;');
}
