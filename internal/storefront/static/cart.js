// 购物车渐进增强 / Cart progressive enhancement
// 作者：仗键天涯(daxing) 3442535897@qq.com  时间：2026-06-18 12:40:00
(function () {
  'use strict';

  function api(method, url, body) {
    var opts = { method: method, credentials: 'same-origin', headers: {} };
    if (body) { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
    return fetch(url, opts).then(function (r) {
      return r.text().then(function (t) {
        var d = t ? JSON.parse(t) : {};
        if (!r.ok) { throw new Error((d && d.error) || ('HTTP ' + r.status)); }
        return d;
      });
    });
  }

  function refreshCount() {
    api('GET', '/cart/data').then(function (d) {
      document.querySelectorAll('[data-cart-count]').forEach(function (el) {
        el.textContent = d.count > 0 ? d.count : '';
      });
    }).catch(function () {});
  }

  function bindAddToCart() {
    var form = document.querySelector('[data-add-form]');
    if (!form) return;
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      var sel = form.querySelector('[name="variant"]:checked') || form.querySelector('[name="variant"]');
      if (!sel) return;
      var qtyEl = form.querySelector('[name="quantity"]');
      var qty = qtyEl ? parseInt(qtyEl.value, 10) || 1 : 1;
      var btn = form.querySelector('button[type="submit"]');
      if (btn) btn.disabled = true;
      api('POST', '/cart/items', { variant: sel.value, quantity: qty })
        .then(function () { refreshCount(); flash(form, '已加入购物车 ✓'); })
        .catch(function (err) { flash(form, err.message, true); })
        .finally(function () { if (btn) btn.disabled = false; });
    });
  }

  function bindCartPage() {
    document.querySelectorAll('[data-cart-qty]').forEach(function (input) {
      input.addEventListener('change', function () {
        var vid = input.getAttribute('data-cart-qty');
        api('PATCH', '/cart/items/' + encodeURIComponent(vid), { quantity: parseInt(input.value, 10) || 0 })
          .then(function () { location.reload(); }).catch(function (e) { alert(e.message); });
      });
    });
    document.querySelectorAll('[data-cart-remove]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        var vid = btn.getAttribute('data-cart-remove');
        api('DELETE', '/cart/items/' + encodeURIComponent(vid))
          .then(function () { location.reload(); }).catch(function (e) { alert(e.message); });
      });
    });
  }

  function flash(form, msg, isErr) {
    var el = form.querySelector('[data-flash]');
    if (!el) return;
    el.textContent = msg;
    el.style.color = isErr ? '#dc2626' : '#059669';
  }

  document.addEventListener('DOMContentLoaded', function () {
    refreshCount();
    bindAddToCart();
    bindCartPage();
  });
})();
