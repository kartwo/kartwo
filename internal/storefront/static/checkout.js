// 结算交互增强 / Checkout Progressive Enhancement
// 功能：更换国家即作废旧确认；完整报价与下单仍由服务端处理，无浏览器持久化客户资料
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-25 18:45:22
(() => {
  const form = document.getElementById('checkout-form');
  if (!form) return;
  const country = form.elements.namedItem('country');
  country.addEventListener('change', () => {
    form.elements.namedItem('quote').value = '';
    document.getElementById('checkout-place').disabled = true;
    document.getElementById('quote-status').textContent = 'Your destination has changed. Review delivery again to update the address requirements and total.';
    const summary = document.querySelector('.checkout-summary');
    // Old totals are clearly marked until a fresh server response replaces the page.
    summary.setAttribute('aria-label', 'Previous quote — review delivery again');
    summary.classList.add('checkout-quote-stale');
  });
  const error = document.getElementById('checkout-error');
  if (error) error.focus();
})();
