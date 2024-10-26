package widgets

import "math"

type Matrix3x3 [3][3]float64

func NewMatrix3x3() Matrix3x3 {
	return Matrix3x3{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}
}

func (m Matrix3x3) Multiply(other Matrix3x3) Matrix3x3 {
	var result Matrix3x3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			sum := 0.0
			for k := 0; k < 3; k++ {
				sum += m[i][k] * other[k][j]
			}
			result[i][j] = sum
		}
	}
	return result
}

func (m Matrix3x3) MultiplyVector(v [3]float64) [3]float64 {
	var result [3]float64
	for i := 0; i < 3; i++ {
		sum := 0.0
		for j := 0; j < 3; j++ {
			sum += m[i][j] * v[j]
		}
		result[i] = sum
	}
	return result
}

func RotationMatrixX(angle float64) Matrix3x3 {
	rad := angle * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)
	return Matrix3x3{
		{1, 0, 0},
		{0, cos, -sin},
		{0, sin, cos},
	}
}

func RotationMatrixY(angle float64) Matrix3x3 {
	rad := angle * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)
	return Matrix3x3{
		{cos, 0, sin},
		{0, 1, 0},
		{-sin, 0, cos},
	}
}

func RotationMatrixZ(angle float64) Matrix3x3 {
	rad := angle * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)
	return Matrix3x3{
		{cos, -sin, 0},
		{sin, cos, 0},
		{0, 0, 1},
	}
}
