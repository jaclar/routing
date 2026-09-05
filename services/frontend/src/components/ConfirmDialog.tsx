import React, { ReactNode } from 'react';
import { AlertTriangle } from 'lucide-react';
import { ModalPortal } from './ModalPortal';

interface ConfirmDialogProps {
  title: string;
  message: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
}

/**
 * A compact confirmation dialog for actions that discard work the user cannot get back without
 * recomputing. Portalled to the body so it centres on the viewport wherever it is declared.
 */
export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  title,
  message,
  confirmLabel = 'Continue',
  cancelLabel = 'Cancel',
  onConfirm,
  onCancel,
}) => (
  <ModalPortal onDismiss={onCancel}>
    <div className="modal-backdrop" onClick={onCancel}>
      <div
        className="confirm-dialog"
        role="alertdialog"
        aria-modal="true"
        aria-label={title}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="confirm-dialog-icon">
          <AlertTriangle size={20} />
        </div>
        <div className="confirm-dialog-content">
          <h3 className="confirm-dialog-title">{title}</h3>
          <div className="confirm-dialog-message">{message}</div>
        </div>
        {/* Cancel takes focus, not confirm: this dialog guards work the user cannot recover
            without recomputing, so a stray Enter should be the harmless outcome. */}
        <div className="confirm-dialog-actions">
          <button type="button" className="btn-modal-cancel" onClick={onCancel} autoFocus>
            {cancelLabel}
          </button>
          <button type="button" className="btn-primary" onClick={onConfirm}>
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  </ModalPortal>
);

export default ConfirmDialog;
