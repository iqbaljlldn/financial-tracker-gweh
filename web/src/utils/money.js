/**
 * Money utilities adhering strictly to 09_api_specification.md §4.2:
 * "Never use binary floating point (parseFloat) for financial values."
 * All values are maintained as exact decimal strings.
 */

const CURRENCY_SYMBOLS = {
  USD: '$',
  EUR: '€',
  GBP: '£',
  IDR: 'Rp ',
  SGD: 'S$',
  JPY: '¥',
};

/**
 * Formats a money object { value: string, currency: string } into a localized string
 * with exact decimal precision, WITHOUT using parseFloat.
 *
 * @param {Object|string} amountObj - { value: "45.00", currency: "USD" } or raw decimal string
 * @param {string} [defaultCurrency="USD"]
 * @returns {string} Formatted display string e.g. "$45.00" or "Rp 50.000,00"
 */
export function formatMoney(amountObj, defaultCurrency = 'USD') {
  if (!amountObj) return '—';

  let valueStr = '';
  let currency = defaultCurrency;

  if (typeof amountObj === 'object') {
    valueStr = String(amountObj.value || '0');
    currency = amountObj.currency || defaultCurrency;
  } else {
    valueStr = String(amountObj);
  }

  // Trim whitespace
  valueStr = valueStr.trim();
  if (!valueStr) return '—';

  // Handle negative sign
  const isNegative = valueStr.startsWith('-');
  if (isNegative) {
    valueStr = valueStr.slice(1);
  }

  // Split into integer and fractional parts
  const parts = valueStr.split('.');
  let integerPart = parts[0] || '0';
  let fractionPart = parts.length > 1 ? parts[1] : '';

  // Pad fraction to at least 2 digits (e.g. "45" -> "45.00", "45.5" -> "45.50")
  if (currency === 'JPY') {
    fractionPart = ''; // Yen doesn't typically show cents
  } else if (fractionPart.length === 0) {
    fractionPart = '00';
  } else if (fractionPart.length === 1) {
    fractionPart += '0';
  }

  // Format integer part with thousands separators using regex (pure string manipulation, no floats)
  const formattedInteger = integerPart.replace(/\B(?=(\d{3})+(?!\d))/g, ',');

  const symbol = CURRENCY_SYMBOLS[currency] || `${currency} `;
  const sign = isNegative ? '-' : '';
  const decimalFormatted = fractionPart ? `${formattedInteger}.${fractionPart}` : formattedInteger;

  return `${sign}${symbol}${decimalFormatted}`;
}

/**
 * Validates whether a value string is a valid positive or negative decimal string.
 *
 * @param {string} val
 * @returns {boolean}
 */
export function isValidDecimalString(val) {
  if (typeof val !== 'string') return false;
  return /^-?\d+(\.\d{1,4})?$/.test(val.trim());
}
