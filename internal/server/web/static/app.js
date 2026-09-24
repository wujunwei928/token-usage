// Theme controller: three states (auto / dark / light) persisted under
// 'tu-theme'. The inline bootstrap in head.html has already resolved the
// preference to data-theme before first paint; this controller cycles the
// stored preference on click, keeps auto mode live against OS theme changes,
// and broadcasts 'tu-themechange' so charts (and anything else) can re-skin.
(function () {
  var KEY = 'tu-theme';
  var ORDER = ['auto', 'dark', 'light'];
  var TITLES = { auto: '主题:跟随系统', dark: '主题:暗色', light: '主题:亮色' };
  var root = document.documentElement;
  var btn = document.getElementById('theme-toggle');

  function resolve(pref) {
    return pref === 'dark' ||
      (pref === 'auto' && matchMedia('(prefers-color-scheme: dark)').matches)
      ? 'dark' : 'light';
  }

  function apply(pref) {
    root.dataset.theme = resolve(pref);
    root.dataset.themePref = pref;
    if (btn) btn.title = TITLES[pref];
    try { localStorage.setItem(KEY, pref); } catch (e) { /* private mode */ }
    window.dispatchEvent(new CustomEvent('tu-themechange', {
      detail: { pref: pref, theme: root.dataset.theme },
    }));
  }

  if (btn) {
    btn.title = TITLES[root.dataset.themePref || 'auto'];
    btn.addEventListener('click', function () {
      var cur = root.dataset.themePref || 'auto';
      apply(ORDER[(ORDER.indexOf(cur) + 1) % ORDER.length]);
    });
  }

  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function () {
    if ((root.dataset.themePref || 'auto') === 'auto') apply('auto');
  });
})();

// Dashboard chart wiring: reads the JSON block the SSR template emits and
// renders it with ECharts. Pages without #dash-data do nothing.
// All colors resolve from the CSS design tokens at runtime, and the charts
// rebuild (dispose + re-init) on 'tu-themechange' so light/dark flip in
// place without another data round-trip.
(function () {
  var block = document.getElementById('dash-data');
  if (!block || typeof echarts === 'undefined') return;
  var data = JSON.parse(block.textContent);

  var instances = [];

  function token(name) {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  }

  function theme() {
    return {
      cats: ['--cat-1', '--cat-2', '--cat-3', '--cat-4', '--cat-5', '--cat-6', '--cat-7', '--cat-8'].map(token),
      text: token('--chart-text'),
      axis: token('--chart-axis'),
      grid: token('--chart-grid'),
      tipBg: token('--tooltip-bg'),
      tipInk: token('--tooltip-ink'),
      tipLine: token('--tooltip-line'),
    };
  }

  function mount(id, option) {
    var el = document.getElementById(id);
    if (!el) return;
    var c = echarts.init(el);
    c.setOption(option);
    instances.push(c);
  }

  function tooltip(t, extra) {
    var tip = {
      backgroundColor: t.tipBg,
      borderColor: t.tipLine,
      textStyle: { color: t.tipInk },
    };
    for (var k in (extra || {})) tip[k] = extra[k];
    return tip;
  }

  // Chinese compact units, twin of Go FormatTokens: ≥1亿 two decimals,
  // ≥1万 one decimal, below stays exact with zh-CN grouping.
  function fmtTokens(n) {
    if (n >= 1e8) return (n / 1e8).toFixed(2) + '亿';
    if (n >= 1e4) return (n / 1e4).toFixed(1) + '万';
    return (+n).toLocaleString('zh-CN');
  }

  function fmtRate(r) {
    return r == null ? '—' : (r * 100).toFixed(2) + '%';
  }

  // Axis-trigger tooltip listing each series with its own unit.
  function axisTip(units) {
    return function (params) {
      if (!params || !params.length) return '';
      var lines = [params[0].axisValueLabel || params[0].name];
      params.forEach(function (p) {
        if (p.value == null) return;
        var unit = units[p.seriesName] || fmtTokens;
        lines.push(p.marker + ' ' + p.seriesName + '  ' + unit(p.value));
      });
      return lines.join('<br/>');
    };
  }

  function valueAxis(t) {
    return {
      type: 'value',
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: t.axis, fontSize: 11, formatter: function (v) { return fmtTokens(v); } },
      splitLine: { lineStyle: { color: t.grid } },
    };
  }

  function categoryAxis(t, names) {
    return {
      type: 'category',
      data: names,
      axisLine: { lineStyle: { color: t.grid } },
      axisTick: { show: false },
      axisLabel: { color: t.axis, fontSize: 11 },
    };
  }

  function legend(t) {
    return { top: 0, textStyle: { color: t.text, fontSize: 12 } };
  }

  function build() {
    instances.forEach(function (c) { c.dispose(); });
    instances = [];
    var t = theme();

    // Hourly timeline: stacked bars per tool.
    var tools = {};
    data.hourly.forEach(function (p) {
      Object.keys(p.Tools).forEach(function (tool) { tools[tool] = true; });
    });
    var toolNames = Object.keys(tools).sort();
    var hours = data.hourly.map(function (p) { return p.Hour + ':00'; });
    mount('chart-hourly', {
      tooltip: tooltip(t, { trigger: 'axis', formatter: axisTip({}) }),
      legend: legend(t),
      grid: { left: 60, right: 20, top: 30, bottom: 30 },
      xAxis: categoryAxis(t, hours),
      yAxis: valueAxis(t),
      series: toolNames.map(function (name, i) {
        return {
          name: name, type: 'bar', stack: 'total', barMaxWidth: 26,
          itemStyle: { color: t.cats[i % t.cats.length] },
          data: data.hourly.map(function (p) { return p.Tools[name] || 0; }),
        };
      }),
    });

    // 30-day tokens + cost dual axis, cache-hit-rate line on a percent axis.
    mount('chart-daily', {
      tooltip: tooltip(t, {
        trigger: 'axis',
        formatter: axisTip({
          cost: function (v) { return '$' + (+v).toFixed(4); },
          '命中率': function (v) { return (+v).toFixed(2) + '%'; },
        }),
      }),
      legend: legend(t),
      grid: { left: 60, right: 100, top: 30, bottom: 30 },
      xAxis: categoryAxis(t, data.daily.map(function (d) { return d.Date.slice(5); })),
      yAxis: [
        Object.assign(valueAxis(t), { name: 'tokens', nameTextStyle: { color: t.axis } }),
        Object.assign(valueAxis(t), {
          name: 'USD', nameTextStyle: { color: t.axis }, splitLine: { show: false },
          axisLabel: { color: t.axis, fontSize: 11, formatter: function (v) { return '$' + v; } },
        }),
        Object.assign(valueAxis(t), {
          name: '命中率', nameTextStyle: { color: t.axis }, splitLine: { show: false }, min: 0, max: 100, offset: 46,
          axisLabel: { color: t.axis, fontSize: 11, formatter: function (v) { return v + '%'; } },
        }),
      ],
      series: [
        {
          name: 'tokens', type: 'bar', barMaxWidth: 22,
          itemStyle: { color: t.cats[0] },
          data: data.daily.map(function (d) { return d.Tokens; }),
        },
        {
          name: 'cost', type: 'line', yAxisIndex: 1, smooth: true,
          itemStyle: { color: t.cats[2] }, lineStyle: { color: t.cats[2] },
          data: data.daily.map(function (d) { return +d.Cost.toFixed(4); }),
        },
        {
          name: '命中率', type: 'line', yAxisIndex: 2, smooth: true,
          itemStyle: { color: t.cats[1] }, lineStyle: { color: t.cats[1] },
          data: data.daily.map(function (d) { return d.HasRate ? +(d.HitRate * 100).toFixed(2) : null; }),
        },
      ],
    });

    function barOf(id, rows, color) {
      mount(id, {
        tooltip: tooltip(t, {
          formatter: function (p) {
            var r = rows[p.dataIndex];
            if (!r) return '';
            var lines = [p.marker + ' ' + r.Name];
            lines.push('Token:' + fmtTokens(r.Tokens));
            if (r.Cost) lines.push('成本:$' + r.Cost.toFixed(4));
            lines.push('命中率:' + fmtRate(r.HasRate ? r.HitRate : null));
            return lines.join('<br/>');
          },
        }),
        grid: { left: 110, right: 20, top: 10, bottom: 30 },
        xAxis: valueAxis(t),
        yAxis: {
          type: 'category',
          data: rows.map(function (r) { return r.Name; }).reverse(),
          axisLine: { lineStyle: { color: t.grid } },
          axisTick: { show: false },
          axisLabel: { color: t.axis, fontSize: 11 },
        },
        series: [{
          type: 'bar', barMaxWidth: 18,
          itemStyle: { color: color, borderRadius: [0, 6, 6, 0] },
          data: rows.map(function (r) { return r.Tokens; }).reverse(),
        }],
      });
    }

    barOf('chart-tool', data.byTool, t.cats[1]);
    barOf('chart-model', data.byModel.slice(0, 8), t.cats[0]);

    mount('chart-compose', {
      tooltip: tooltip(t, {
        trigger: 'item',
        formatter: function (p) {
          return p.marker + ' ' + p.name + ':' + fmtTokens(p.value);
        },
      }),
      series: [{
        type: 'pie', radius: ['40%', '70%'],
        label: { color: t.text, formatter: '{b}\n{d}%' },
        data: [
          { name: '输入', value: data.compose.Input, itemStyle: { color: t.cats[0] } },
          { name: '输出', value: data.compose.Output, itemStyle: { color: t.cats[1] } },
          { name: '缓存读', value: data.compose.CacheRead, itemStyle: { color: t.cats[2] } },
          { name: '缓存写 5m', value: data.compose.Cache5m, itemStyle: { color: t.cats[3] } },
          { name: '缓存写 1h', value: data.compose.Cache1h, itemStyle: { color: t.cats[5] } },
        ].filter(function (d) { return d.value > 0; }),
      }],
    });

    // Weekday × hour heatmap of the all-time working rhythm.
    var wdNames = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'];
    var heat = (data.heatmap || []).map(function (c) { return [c.Hour, c.Weekday, c.Tokens]; });
    var heatMax = heat.reduce(function (m, c) { return Math.max(m, c[2]); }, 0) || 1;
    mount('chart-heatmap', {
      tooltip: tooltip(t, {
        formatter: function (p) {
          return wdNames[p.value[1]] + ' ' + p.value[0] + ':00 · ' + fmtTokens(p.value[2]);
        },
      }),
      grid: { left: 44, right: 10, top: 8, bottom: 60 },
      xAxis: {
        type: 'category',
        data: Array.from({ length: 24 }, function (_, h) { return h; }),
        axisLine: { lineStyle: { color: t.grid } },
        axisTick: { show: false },
        axisLabel: { color: t.axis, fontSize: 10, interval: 1 },
        splitArea: { show: true, areaStyle: { color: [t.tipBg, 'transparent'] } },
      },
      yAxis: {
        type: 'category',
        data: wdNames,
        axisLine: { lineStyle: { color: t.grid } },
        axisTick: { show: false },
        axisLabel: { color: t.axis, fontSize: 11 },
      },
      visualMap: {
        min: 0, max: heatMax, calculable: false,
        orient: 'horizontal', left: 'center', bottom: 0, itemWidth: 12, itemHeight: 90,
        textStyle: { color: t.axis, fontSize: 10 },
        formatter: function (v) { return fmtTokens(+v); },
        inRange: { color: [t.grid, t.cats[5], t.cats[1]] },
      },
      series: [{
        type: 'heatmap',
        data: heat,
        itemStyle: { borderColor: t.tipBg, borderWidth: 1 },
      }],
    });
  }

  build();
  window.addEventListener('tu-themechange', build);

  window.addEventListener('resize', function () {
    document.querySelectorAll('.chart').forEach(function (el) {
      var inst = echarts.getInstanceByDom(el);
      if (inst) inst.resize();
    });
  });
})();
