var chart_1 = Highcharts.chart('chart-1', {
	chart: {
		type: 'scatter',
		margin: [70, 50, 60, 80],
	},
	title: {
		text: 'Chart title',
		align: 'left'
	},
	subtitle: {
		text: 'Some description?',
		align: 'left'
	},
	time: {
		useUTC: false
	},
	tooltip: {
		headerFormat: '<b>{series.name}</b><br/>',
		pointFormat: '{point.x:%Y-%m-%d %H:%M:%S}<br/>{point.y:.2f}'
	},
	xAxis: {
		gridLineWidth: 1,
		minPadding: 0.2,
		maxPadding: 0.2,
		maxZoom: 60
	},
	yAxis: {
		title: {
			text: 'Value'
		},
		minPadding: 0.2,
		maxPadding: 0.2,
		maxZoom: 60,
		plotLines: [{
			value: 0,
			width: 1,
			color: '#808080'
		}]
	},
	legend: {
		enabled: true
	},
	plotOptions: {
		series: {
			lineWidth: 1,
		}
	},
	series: [{
		data: [[20, 20], [80, 80]]
	}]
});

var chart_2 = Highcharts.chart('chart-2', {
	chart: {
		type: 'spline',
		animation: Highcharts.svg, // don't animate in old IE
		marginRight: 10,
	},

	time: {
		useUTC: false
	},

	title: {
		text: 'Live random data'
	},

	xAxis: {
		type: 'datetime',
		tickPixelInterval: 150
	},

	yAxis: {
		title: {
			text: 'Value'
		},
		plotLines: [{
			value: 0,
			width: 1,
			color: '#808080'
		}]
	},

	tooltip: {
		headerFormat: '<b>{series.name}</b><br/>',
		pointFormat: '{point.x:%Y-%m-%d %H:%M:%S}<br/>{point.y:.2f}'
	},

	legend: {
		enabled: false
	},

	exporting: {
		enabled: false
	},

	series: [{
		name: 'Random data',
		data: (function () {
			// generate an array of random data
			var data = [],
				time = (new Date()).getTime(),
				i;

			for (i = -19; i <= 0; i += 1) {
				data.push({
					x: time + i * 1000,
					y: Math.random()
				});
			}
			return data;
		}())
	}]
});

setInterval(() => {
	var x = (new Date()).getTime(), // current time
		y = Math.random();
	chart_1.series[0].addPoint([x, y], true, true)
}, 700);
setInterval(function () {
	var x = (new Date()).getTime(), // current time
		y = Math.random();
	chart_2.series[0].addPoint([x, y], true, true);
}, 1000);