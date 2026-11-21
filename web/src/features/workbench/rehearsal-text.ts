/** The bytes a text takes as UTF-8, the unit every server bound on typed text is stated in. */
export const textBytes = (text: string) => new TextEncoder().encode(text).length;

/** Whether a text fits a bound stated in UTF-8 bytes. */
export const fitsIn = (maxBytes: number) => (text: string) => textBytes(text) <= maxBytes;
