import React, { ReactNode, useEffect } from 'react';
import { createPortal } from 'react-dom';

interface ModalPortalProps {
  children: ReactNode;
  /** Called on Escape. Omit for a dialog that must be dismissed explicitly. */
  onDismiss?: () => void;
}

/**
 * Renders a dialog into document.body.
 *
 * Dialogs declared inside the map overlays cannot position themselves against the viewport:
 * `.timeline-bar` and the other floating panels use `transform` and `backdrop-filter`, and either
 * property makes an element the containing block for its `position: fixed` descendants. A
 * full-screen backdrop nested in the bottom dock is therefore laid out inside that dock rather
 * than the viewport, which pushes the dialog off the bottom of the screen. Portalling to the body
 * sidesteps the whole problem, so `position: fixed` means what it says.
 */
export const ModalPortal: React.FC<ModalPortalProps> = ({ children, onDismiss }) => {
  useEffect(() => {
    if (!onDismiss) return;
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onDismiss();
    };
    window.addEventListener('keydown', handleKey);
    return () => window.removeEventListener('keydown', handleKey);
  }, [onDismiss]);

  return createPortal(children, document.body);
};

export default ModalPortal;
