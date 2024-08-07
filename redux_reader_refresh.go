package kevlar

// assetModTimes returns ModTime for each asset. It doesn't update it
// because that time should be updated only when asset is loaded
func (rdx *redux) assetsModTimes() (map[string]int64, error) {
	amts := make(map[string]int64)
	var err error
	for asset := range rdx.akv {
		amts[asset], err = rdx.kv.ModTime(asset)
		if err != nil {
			return nil, err
		}
	}

	return amts, nil
}

func (rdx *redux) ModTime() (int64, error) {
	var mt int64 = -1
	amts, err := rdx.assetsModTimes()
	if err != nil {
		return -1, err
	}

	for asset := range rdx.akv {
		if amt, ok := amts[asset]; ok && amt > mt {
			mt = amt
		}
	}

	return mt, nil
}

func (rdx *redux) refresh() (*redux, error) {

	amts, err := rdx.assetsModTimes()
	if err != nil {
		return nil, err
	}

	for asset := range rdx.akv {
		// asset was updated externally
		if rdx.lmt[asset] < amts[asset] {
			ckv, err := loadAsset(rdx.kv, asset)
			if err != nil {
				return nil, err
			}
			rdx.akv[asset] = ckv
			rdx.lmt[asset] = amts[asset]
		}
	}

	return rdx, nil
}

func (rdx *redux) RefreshReader() (ReadableRedux, error) {
	return rdx.refresh()
}
