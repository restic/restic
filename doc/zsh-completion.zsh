#compdef restic

_arguments \
  '1: :->level1' \
  '2: :_files'
case $state in
  level1)
    case $words[1] in
      restic)
        _arguments '1: :(backup cache cat check diff dump find forget generate help init key list ls migrate mount options prune rebuild-index recover restore self-update snapshots stats tag unlock version)'
      ;;
      *)
        _arguments '*: :_files'
      ;;
    esac
  ;;
  *)
    _arguments '*: :_files'
  ;;
esac
