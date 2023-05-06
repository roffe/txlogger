var socket = io("ws://localhost:8080");

socket.on("symbol_list", data => {
	console.log(data);
});
socket.on('connect', () => {
	console.log('connect');
	$('#loading-spinner').remove();
	$('#container>.chart').remove();
	socket.emit('start_session');
});






// var chart_1 = Highcharts.chart('chart-1', {
// 	chart: {
// 		type: 'scatter',
// 		zoomType: 'x',
// 		animation: Highcharts.svg, // don't animate in old IE
// 		marginRight: 10,
// 	},
// 	title: {
// 		text: 'ActualIn.T_Engine',
// 		align: 'left'
// 	},
// 	subtitle: {
// 		text: 'Some description?',
// 		align: 'left'
// 	},
// 	time: {
// 		useUTC: false
// 	},
// 	tooltip: {
// 		headerFormat: '<b>{series.name}</b><br/>',
// 		pointFormat: '{point.x:%Y-%m-%d %H:%M:%S}<br/>{point.y:.2f}'
// 	},
// 	xAxis: {
// 		type: 'datetime',
// 		tickPixelInterval: 150
// 	},
// 	yAxis: {
// 		title: {
// 			text: 'Value'
// 		},
// 		plotLines: [{
// 			value: 0,
// 			width: 1,
// 			color: '#808080'
// 		}]
// 	},
// 	legend: {
// 		enabled: true
// 	},
// 	plotOptions: {
// 		series: {
// 			lineWidth: 1,
// 		}
// 	},
// 	series: [{
// 		name: 'ActualIn.T_Engine',
// 		data: []
// 	}]
// });

// var chart_2 = Highcharts.chart('chart-2', {
// 	chart: {
// 		type: 'spline',
// 		zoomType: 'x',
// 		animation: Highcharts.svg, // don't animate in old IE
// 		marginRight: 10,
// 	},

// 	time: {
// 		useUTC: false
// 	},

// 	title: {
// 		text: 'ActualIn.T_AirInlet'
// 	},
// 	xAxis: {
// 		type: 'datetime',
// 		tickPixelInterval: 150
// 	},
// 	yAxis: {
// 		title: {
// 			text: 'Value'
// 		},
// 		plotLines: [{
// 			value: 0,
// 			width: 1,
// 			color: '#808080'
// 		}]
// 	},
// 	tooltip: {
// 		headerFormat: '<b>{series.name}</b><br/>',
// 		pointFormat: '{point.x:%Y-%m-%d %H:%M:%S}<br/>{point.y:.2f}'
// 	},
// 	series: [{
// 		name: 'ActualIn.T_AirInlet',
// 		data: []
// 	}]
// });

// setInterval(() => {
// 	var shift = false;
// 	var x = (new Date()).getTime(); // current time
// 	chart_1.series[0].addPoint([x, Math.random()], true, shift)
// 	chart_2.series[0].addPoint([x, Math.random()], true, shift);
// }, 1000);