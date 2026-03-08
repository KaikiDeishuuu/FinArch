import { useState } from "react";
import {
  executeRestore,
  restoreBackup,
  sendRestoreVerification,
  verifyRestoreCode,
} from "../api/client";

interface RestoreVerificationState {
  open: boolean;
  restoreId: string;
  maskedEmail: string;
  emailSent: boolean;
  pendingFile: File | null;
}

export function useRestoreBackup(
  onRestoreSuccess: (result: {
    restored_version: number;
    migrated_to: number;
  }) => Promise<void> | void,
) {
  const [loading, setLoading] = useState(false);
  const [verification, setVerification] = useState<RestoreVerificationState>({
    open: false,
    restoreId: "",
    maskedEmail: "",
    emailSent: false,
    pendingFile: null,
  });

  const requestRestore = async (file: File) => {
    setLoading(true);
    try {
      const result = await restoreBackup(file);
      await onRestoreSuccess(result);
      return { status: "restored" as const, result };
    } catch (err: any) {
      const code =
        err?.response?.data?.error?.code ?? err?.response?.data?.code;
      if (code === "RESTORE_VERIFICATION_REQUIRED") {
        setVerification({
          open: true,
          restoreId: "",
          maskedEmail: "",
          emailSent: false,
          pendingFile: file,
        });
        return { status: "verification_required" as const };
      }
      throw err;
    } finally {
      setLoading(false);
    }
  };

  const sendEmailCode = async (originalEmail: string, file?: File) => {
    setLoading(true);
    try {
      const selectedFile = file ?? verification.pendingFile;
      if (!selectedFile) throw new Error("restore file missing");
      const resp = await sendRestoreVerification(selectedFile, originalEmail);
      setVerification({
        open: true,
        restoreId: resp.restore_id,
        maskedEmail: resp.masked_email,
        emailSent: true,
        pendingFile: selectedFile,
      });
      return resp;
    } finally {
      setLoading(false);
    }
  };

  const submitCode = async (code: string) => {
    if (!verification.restoreId) {
      throw new Error("restore id missing");
    }
    setLoading(true);
    try {
      const verified = await verifyRestoreCode(verification.restoreId, code);
      const restored = await executeRestore(verified.restore_token);
      await onRestoreSuccess(restored);
      setVerification({
        open: false,
        restoreId: "",
        maskedEmail: "",
        emailSent: false,
        pendingFile: null,
      });
      return restored;
    } finally {
      setLoading(false);
    }
  };

  return {
    loading,
    verification,
    closeVerification: () =>
      setVerification({
        open: false,
        restoreId: "",
        maskedEmail: "",
        emailSent: false,
        pendingFile: null,
      }),
    requestRestore,
    sendEmailCode,
    submitCode,
  };
}
