package utils

// IsIntInSlice 判断数字是否在切片中
func IsIntInSlice(value int, list []int) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// RemoveValueInIntSlice 从int切片中删除指定值
func RemoveValueInIntSlice(slice []int, value int) []int {
	// 找到值在切片中的索引
	index := -1
	for i, v := range slice {
		if v == value {
			index = i
			break
		}
	}

	// 如果找到了值，从切片中删除
	if index != -1 {
		slice = append(slice[:index], slice[index+1:]...)
	}
	return slice
}
